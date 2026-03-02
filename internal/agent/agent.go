package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"

	"github.com/company4312/copilot-telegram-bot/internal/api"
	"github.com/company4312/copilot-telegram-bot/internal/store"
)

// sessionKey uniquely identifies a session by agent name and chat ID.
type sessionKey struct {
	agent  string
	chatID int64
}

// Pool manages multiple named agents and their Copilot sessions.
type Pool struct {
	client     *copilot.Client
	store      *store.Store
	configDir  string
	sessions   map[sessionKey]*copilot.Session
	mu         sync.Mutex
	apiServer  *api.Server
	notifyFunc func(chatID int64, text string) // callback to send messages to a chat
}

// SetAPIServer sets the API server used for broadcasting activity events.
func (p *Pool) SetAPIServer(srv *api.Server) {
	p.apiServer = srv
}

// SetNotifyFunc sets the callback used to send messages to a Telegram chat.
func (p *Pool) SetNotifyFunc(fn func(chatID int64, text string)) {
	p.notifyFunc = fn
}

// logActivity records an activity entry and broadcasts it to SSE clients.
func (p *Pool) logActivity(agentName, eventType, content, metadata string, chatID int64) {
	entry := store.ActivityEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		AgentName: agentName,
		EventType: eventType,
		Content:   content,
		Metadata:  metadata,
		ChatID:    chatID,
	}
	if err := p.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	if p.apiServer != nil {
		p.apiServer.Broadcast(entry)
	}
}

// jsonMeta safely builds a JSON metadata string from key-value pairs.
func jsonMeta(kv map[string]string) string {
	b, _ := json.Marshal(kv)
	return string(b)
}

// NewPool creates a new agent pool backed by the given store.
func NewPool(s *store.Store, configDir string) *Pool {
	client := copilot.NewClient(&copilot.ClientOptions{
		LogLevel: "error",
	})
	return &Pool{
		client:    client,
		store:     s,
		configDir: configDir,
		sessions:  make(map[sessionKey]*copilot.Session),
	}
}

// Start launches the Copilot CLI server process.
func (p *Pool) Start(ctx context.Context) error {
	return p.client.Start(ctx)
}

// Stop gracefully shuts down the Copilot CLI server and all sessions.
func (p *Pool) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, session := range p.sessions {
		_ = session.Destroy()
		delete(p.sessions, key)
	}
	return p.client.Stop()
}

// SendMessage sends a user message to the active agent for the given chat
// and returns the assistant's full response text.
func (p *Pool) SendMessage(ctx context.Context, chatID int64, text string) (string, error) {
	agentName, err := p.store.GetActiveAgent(chatID)
	if err != nil {
		return "", fmt.Errorf("get active agent: %w", err)
	}
	return p.SendMessageTo(ctx, agentName, chatID, text)
}

// SendMessageTo sends a user message to a specific named agent for the given chat.
func (p *Pool) SendMessageTo(ctx context.Context, agentName string, chatID int64, text string) (string, error) {
	session, err := p.getOrCreateSession(ctx, agentName, chatID)
	if err != nil {
		return "", fmt.Errorf("session setup: %w", err)
	}

	p.logActivity(agentName, "message_sent", text, "", chatID)

	response, err := p.sendAndWait(ctx, session, text)
	if err != nil {
		// If send fails, the session may be stale — clear it and retry once.
		p.clearSession(agentName, chatID)
		session, err = p.getOrCreateSession(ctx, agentName, chatID)
		if err != nil {
			return "", fmt.Errorf("session retry: %w", err)
		}
		response, err = p.sendAndWait(ctx, session, text)
		if err != nil {
			p.logActivity(agentName, "error", err.Error(), "", chatID)
			return "", fmt.Errorf("send message: %w", err)
		}
	}

	p.logActivity(agentName, "message_received", response, "", chatID)

	return response, nil
}

// SendMessageBetween delivers a message from one agent to another within the same chat.
// The message is prefixed with the sender's identity so the recipient knows who is talking.
func (p *Pool) SendMessageBetween(ctx context.Context, fromAgent, toAgent string, chatID int64, text string) (string, error) {
	fromCfg, err := p.store.GetAgent(fromAgent)
	if err != nil || fromCfg == nil {
		return "", fmt.Errorf("unknown sender agent: %s", fromAgent)
	}
	prefixed := fmt.Sprintf("[Message from %s (%s)]\n\n%s", fromCfg.Name, fromCfg.Title, text)
	p.logActivity(fromAgent, "agent_message", text, jsonMeta(map[string]string{"to": toAgent}), chatID)
	return p.SendMessageTo(ctx, toAgent, chatID, prefixed)
}

// sendAndWait sends a prompt on the session and blocks until "session.idle".
func (p *Pool) sendAndWait(ctx context.Context, session *copilot.Session, text string) (string, error) {
	var (
		response string
		done     = make(chan struct{})
		once     sync.Once
	)

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case "assistant.message":
			if event.Data.Content != nil {
				response = *event.Data.Content
			}
		case "session.idle":
			once.Do(func() { close(done) })
		}
	})
	defer unsubscribe()

	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: text}); err != nil {
		return "", err
	}

	select {
	case <-done:
		return response, nil
	case <-ctx.Done():
		return response, ctx.Err()
	}
}

// ResetSession destroys and removes the session for the active agent in a chat.
func (p *Pool) ResetSession(ctx context.Context, chatID int64) error {
	agentName, err := p.store.GetActiveAgent(chatID)
	if err != nil {
		return fmt.Errorf("get active agent: %w", err)
	}
	p.clearSession(agentName, chatID)
	return p.store.DeleteSession(chatID, agentName)
}

// getOrCreateSession returns the cached session or creates/resumes one.
func (p *Pool) getOrCreateSession(ctx context.Context, agentName string, chatID int64) (*copilot.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := sessionKey{agent: agentName, chatID: chatID}

	if session, ok := p.sessions[key]; ok {
		return session, nil
	}

	// Look up agent config.
	cfg, err := p.store.GetAgent(agentName)
	if err != nil {
		return nil, fmt.Errorf("get agent config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("unknown agent: %s", agentName)
	}

	// Try to resume a persisted session.
	sessionID, err := p.store.GetSessionID(chatID, agentName)
	if err != nil {
		return nil, err
	}

	if sessionID != "" {
		session, err := p.client.ResumeSession(ctx, sessionID, &copilot.ResumeSessionConfig{
			OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
			ConfigDir:           p.configDir,
		})
		if err == nil {
			p.sessions[key] = session
			return session, nil
		}
		log.Printf("failed to resume session %s for agent %s chat %d, creating new: %v", sessionID, agentName, chatID, err)
	}

	// Create a fresh session.
	systemPrompt := cfg.SystemPrompt

	// Inject agent memories into the system prompt.
	memories, err := p.store.GetMemoriesForPrompt(agentName)
	if err != nil {
		log.Printf("failed to load memories for agent %s: %v", agentName, err)
	} else if memories != "" {
		systemPrompt += "\n\n" + memories
	}

	session, err := p.client.CreateSession(ctx, &copilot.SessionConfig{
		Model:               cfg.Model,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		ConfigDir:           p.configDir,
		SystemMessage: &copilot.SystemMessageConfig{
			Content: systemPrompt,
		},
	})
	if err != nil {
		return nil, err
	}

	if err := p.store.SaveSession(chatID, agentName, session.SessionID); err != nil {
		log.Printf("failed to persist session for agent %s chat %d: %v", agentName, chatID, err)
	}

	p.sessions[key] = session
	p.logActivity(agentName, "session_created", fmt.Sprintf("session %s created", session.SessionID), "", chatID)
	return session, nil
}

// clearSession removes a session from the in-memory cache and destroys it.
func (p *Pool) clearSession(agentName string, chatID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := sessionKey{agent: agentName, chatID: chatID}
	if session, ok := p.sessions[key]; ok {
		_ = session.Destroy()
		delete(p.sessions, key)
		p.logActivity(agentName, "session_destroyed", "session destroyed", "", chatID)
	}
}

// SaveMemory stores a memory for an agent, logs activity, and broadcasts.
func (p *Pool) SaveMemory(agentName, category, content, source string) (int64, error) {
	id, err := p.store.SaveMemory(store.Memory{
		AgentName: agentName,
		Category:  category,
		Content:   content,
		Source:    source,
	})
	if err != nil {
		return 0, err
	}
	p.logActivity(agentName, "memory_created", content, jsonMeta(map[string]string{"category": category, "source": source}), 0)
	return id, nil
}

// CreateProject creates a new project on behalf of an agent.
func (p *Pool) CreateProject(agentName, id, name, description string) error {
	if err := p.store.CreateProject(store.Project{
		ID:          id,
		Name:        name,
		Description: description,
		Status:      "active",
		CreatedBy:   agentName,
	}); err != nil {
		return err
	}
	p.logActivity(agentName, "project_created", fmt.Sprintf("created project %s: %s", id, name), jsonMeta(map[string]string{"project_id": id}), 0)
	return nil
}

// CreateTask creates a new task in a project on behalf of an agent.
func (p *Pool) CreateTask(agentName, projectID, taskID, title, description string, priority int) error {
	if err := p.store.CreateTask(store.Task{
		ID:          taskID,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Status:      "backlog",
		CreatedBy:   agentName,
		Priority:    priority,
	}); err != nil {
		return err
	}
	p.logActivity(agentName, "task_created", fmt.Sprintf("created task %s: %s", taskID, title), jsonMeta(map[string]string{"project_id": projectID, "task_id": taskID}), 0)
	return nil
}

// UpdateTaskStatus updates a task's status, logs activity, and adds a comment.
func (p *Pool) UpdateTaskStatus(agentName, taskID, newStatus string) error {
	if !store.ValidTaskStatuses[newStatus] {
		return fmt.Errorf("invalid task status: %s", newStatus)
	}
	if err := p.store.UpdateTaskStatus(taskID, newStatus); err != nil {
		return err
	}
	comment := fmt.Sprintf("status changed to %s by %s", newStatus, agentName)
	_, _ = p.store.AddTaskComment(store.TaskComment{
		TaskID:    taskID,
		AgentName: agentName,
		Content:   comment,
	})
	p.logActivity(agentName, "task_status_changed", comment, jsonMeta(map[string]string{"task_id": taskID, "status": newStatus}), 0)
	return nil
}

// AssignTask assigns a task to an agent and logs the activity.
func (p *Pool) AssignTask(agentName, taskID, assignee string) error {
	t, err := p.store.GetTask(taskID)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	t.AssignedTo = &assignee
	if err := p.store.UpdateTask(*t); err != nil {
		return err
	}
	p.logActivity(agentName, "task_assigned", fmt.Sprintf("assigned task %s to %s", taskID, assignee), jsonMeta(map[string]string{"task_id": taskID, "assignee": assignee}), 0)
	return nil
}

// CommentOnTask adds a comment on a task and logs the activity.
func (p *Pool) CommentOnTask(agentName, taskID, comment string) error {
	_, err := p.store.AddTaskComment(store.TaskComment{
		TaskID:    taskID,
		AgentName: agentName,
		Content:   comment,
	})
	if err != nil {
		return err
	}
	p.logActivity(agentName, "task_comment", comment, jsonMeta(map[string]string{"task_id": taskID}), 0)
	return nil
}

// DelegateTask creates a task, assigns it to an agent, sends them instructions
// in a background goroutine, and notifies the originating chat when complete.
func (p *Pool) DelegateTask(ctx context.Context, fromAgent, toAgent string, chatID int64, projectID, taskID, title, instructions string, priority int) error {
	// Create the task.
	if err := p.CreateTask(fromAgent, projectID, taskID, title, instructions, priority); err != nil {
		return fmt.Errorf("create task: %w", err)
	}

	// Assign it.
	if err := p.AssignTask(fromAgent, taskID, toAgent); err != nil {
		return fmt.Errorf("assign task: %w", err)
	}

	// Update status to in_progress.
	if err := p.UpdateTaskStatus(fromAgent, taskID, "in_progress"); err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	// Run the agent work asynchronously.
	go func() {
		workCtx := context.Background()
		prompt := fmt.Sprintf("[Task Assignment: %s]\nTask ID: %s\nProject: %s\nPriority: %d\n\nInstructions:\n%s\n\nPlease complete this task. When done, summarize what you did.",
			title, taskID, projectID, priority, instructions)

		response, err := p.SendMessageBetween(workCtx, fromAgent, toAgent, chatID, prompt)
		if err != nil {
			log.Printf("delegate task %s to %s failed: %v", taskID, toAgent, err)
			_ = p.UpdateTaskStatus(fromAgent, taskID, "backlog")
			_ = p.CommentOnTask(fromAgent, taskID, fmt.Sprintf("delegation failed: %v", err))
			if p.notifyFunc != nil {
				p.notifyFunc(chatID, fmt.Sprintf("❌ Task %s failed to delegate to %s: %v", taskID, toAgent, err))
			}
			return
		}

		// Mark as review.
		_ = p.UpdateTaskStatus(toAgent, taskID, "review")
		_ = p.CommentOnTask(toAgent, taskID, response)

		// Notify the originating chat.
		if p.notifyFunc != nil {
			summary := response
			if len(summary) > 500 {
				summary = summary[:500] + "…"
			}
			p.notifyFunc(chatID, fmt.Sprintf("✅ Task *%s* completed by %s:\n\n%s", title, toAgent, summary))
		}
	}()

	return nil
}

// GetTaskSummary returns a formatted status report of active tasks.
func (p *Pool) GetTaskSummary() (string, error) {
	inProgress, err := p.store.ListTasks(store.TaskFilter{Status: "in_progress", Limit: 20})
	if err != nil {
		return "", err
	}
	review, err := p.store.ListTasks(store.TaskFilter{Status: "review", Limit: 20})
	if err != nil {
		return "", err
	}
	todo, err := p.store.ListTasks(store.TaskFilter{Status: "todo", Limit: 10})
	if err != nil {
		return "", err
	}

	if len(inProgress) == 0 && len(review) == 0 && len(todo) == 0 {
		return "No active tasks.", nil
	}

	var sb strings.Builder
	sb.WriteString("📊 *Task Status*\n\n")

	if len(inProgress) > 0 {
		sb.WriteString("🔨 *In Progress:*\n")
		for _, t := range inProgress {
			assignee := "unassigned"
			if t.AssignedTo != nil {
				assignee = *t.AssignedTo
			}
			sb.WriteString(fmt.Sprintf("  • %s → %s\n", t.Title, assignee))
		}
		sb.WriteString("\n")
	}

	if len(review) > 0 {
		sb.WriteString("👀 *In Review:*\n")
		for _, t := range review {
			assignee := "unassigned"
			if t.AssignedTo != nil {
				assignee = *t.AssignedTo
			}
			sb.WriteString(fmt.Sprintf("  • %s → %s\n", t.Title, assignee))
		}
		sb.WriteString("\n")
	}

	if len(todo) > 0 {
		sb.WriteString("📋 *Up Next:*\n")
		for _, t := range todo {
			sb.WriteString(fmt.Sprintf("  • %s\n", t.Title))
		}
	}

	return sb.String(), nil
}

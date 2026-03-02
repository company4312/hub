package agent

import (
	"context"
	"fmt"
	"log"
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
	client    *copilot.Client
	store     *store.Store
	configDir string
	sessions  map[sessionKey]*copilot.Session
	mu        sync.Mutex
	apiServer *api.Server
}

// SetAPIServer sets the API server used for broadcasting activity events.
func (p *Pool) SetAPIServer(srv *api.Server) {
	p.apiServer = srv
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
	p.logActivity(fromAgent, "agent_message", text, fmt.Sprintf(`{"to":"%s"}`, toAgent), chatID)
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
	p.logActivity(agentName, "memory_created", content, fmt.Sprintf(`{"category":"%s","source":"%s"}`, category, source), 0)
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
	p.logActivity(agentName, "project_created", fmt.Sprintf("created project %s: %s", id, name), fmt.Sprintf(`{"project_id":"%s"}`, id), 0)
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
	p.logActivity(agentName, "task_created", fmt.Sprintf("created task %s: %s", taskID, title), fmt.Sprintf(`{"project_id":"%s","task_id":"%s"}`, projectID, taskID), 0)
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
	p.logActivity(agentName, "task_status_changed", comment, fmt.Sprintf(`{"task_id":"%s","status":"%s"}`, taskID, newStatus), 0)
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
	p.logActivity(agentName, "task_assigned", fmt.Sprintf("assigned task %s to %s", taskID, assignee), fmt.Sprintf(`{"task_id":"%s","assignee":"%s"}`, taskID, assignee), 0)
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
	p.logActivity(agentName, "task_comment", comment, fmt.Sprintf(`{"task_id":"%s"}`, taskID), 0)
	return nil
}

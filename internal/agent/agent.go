package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/google/uuid"

	"github.com/company4312/copilot-telegram-bot/internal/store"
)

// memoryMarkerRe matches [MEMORY category="..." source="..."]...[/MEMORY] blocks.
var memoryMarkerRe = regexp.MustCompile(`(?s)\[MEMORY\s+category="([^"]+)"\s+source="([^"]+)"\](.*?)\[/MEMORY\]`)

// sessionKey uniquely identifies a session by agent name and thread ID.
type sessionKey struct {
	agent    string
	threadID string
}

// BroadcastFunc is the callback signature for broadcasting new messages to SSE clients.
type BroadcastFunc func(msg store.Message)

// Pool manages multiple named agents and their Copilot sessions.
type Pool struct {
	client        *copilot.Client
	store         *store.Store
	configDir     string
	sessions      map[sessionKey]*copilot.Session
	mu            sync.Mutex
	notifyFunc    func(chatID int64, text string)
	broadcastFunc BroadcastFunc
}

// SetNotifyFunc sets the callback used to send messages to a Telegram chat.
func (p *Pool) SetNotifyFunc(fn func(chatID int64, text string)) {
	p.notifyFunc = fn
}

// SetBroadcastFunc sets the callback used to broadcast new messages to SSE clients.
func (p *Pool) SetBroadcastFunc(fn BroadcastFunc) {
	p.broadcastFunc = fn
}

// broadcast sends a message to SSE clients if a broadcast function is set.
func (p *Pool) broadcast(msg store.Message) {
	if p.broadcastFunc != nil {
		p.broadcastFunc(msg)
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

// HandleUserMessage routes a Telegram message to the entry-point agent (CTO).
// It reuses the active thread for the chat if one exists, otherwise creates a new one.
func (p *Pool) HandleUserMessage(ctx context.Context, chatID int64, text string) (string, string, error) {
	// Try to reuse the active thread for this chat.
	existing, err := p.store.GetActiveThread(chatID)
	if err != nil {
		return "", "", fmt.Errorf("get active thread: %w", err)
	}

	var threadID string
	if existing != nil {
		threadID = existing.ID
		_ = p.store.TouchThread(threadID)
	} else {
		// Create a new thread.
		threadID = uuid.New().String()
		title := text
		if len(title) > 80 {
			title = title[:80] + "…"
		}
		if err := p.store.CreateThread(store.Thread{
			ID:     threadID,
			ChatID: chatID,
			Title:  title,
			Status: "active",
		}); err != nil {
			return "", "", fmt.Errorf("create thread: %w", err)
		}
	}

	// Add user message.
	userMsgID, err := p.store.AddMessage(store.Message{
		ThreadID:    threadID,
		FromName:    "user",
		Content:     text,
		MessageType: "user_message",
	})
	if err != nil {
		return "", "", fmt.Errorf("add user message: %w", err)
	}

	// Broadcast user message.
	if userMsg, err := p.store.GetMessage(userMsgID); err == nil && userMsg != nil {
		p.broadcast(*userMsg)
	}

	// Route to CTO (entry-point agent).
	agentName := "cto"
	response, responseMsgID, err := p.routeToAgent(ctx, threadID, agentName, text, &userMsgID)
	if err != nil {
		// Retry once on stale session.
		p.clearSession(agentName, threadID)
		response, responseMsgID, err = p.routeToAgent(ctx, threadID, agentName, text, &userMsgID)
		if err != nil {
			return threadID, "", fmt.Errorf("send to agent: %w", err)
		}
	}

	_ = responseMsgID // used for future features
	return threadID, response, nil
}

// InvokeAgent sends a message from one agent to another within a thread.
// This is the core inter-agent communication primitive.
func (p *Pool) InvokeAgent(ctx context.Context, threadID, fromAgent, toAgent, text string, parentMsgID *int64) (string, error) {
	// Record the invocation message.
	invokeMsgID, err := p.store.AddMessage(store.Message{
		ThreadID:        threadID,
		FromName:        fromAgent,
		ToName:          toAgent,
		Content:         text,
		MessageType:     "agent_message",
		ParentMessageID: parentMsgID,
	})
	if err != nil {
		return "", fmt.Errorf("add invoke message: %w", err)
	}
	if invokeMsg, err := p.store.GetMessage(invokeMsgID); err == nil && invokeMsg != nil {
		p.broadcast(*invokeMsg)
	}

	// Prefix the message with sender identity.
	fromCfg, err := p.store.GetAgent(fromAgent)
	if err != nil || fromCfg == nil {
		return "", fmt.Errorf("unknown sender agent: %s", fromAgent)
	}
	prefixed := fmt.Sprintf("[Message from %s (%s)]\n\n%s", fromCfg.Name, fromCfg.Title, text)

	// Route to the target agent.
	response, _, err := p.routeToAgent(ctx, threadID, toAgent, prefixed, &invokeMsgID)
	if err != nil {
		// Retry once on stale session.
		p.clearSession(toAgent, threadID)
		response, _, err = p.routeToAgent(ctx, threadID, toAgent, prefixed, &invokeMsgID)
		if err != nil {
			return "", fmt.Errorf("send to agent %s: %w", toAgent, err)
		}
	}

	return response, nil
}

// routeToAgent sends a prompt to an agent's session, records the response as a message,
// and returns the response text and message ID.
func (p *Pool) routeToAgent(ctx context.Context, threadID, agentName, prompt string, parentMsgID *int64) (string, int64, error) {
	session, err := p.getOrCreateSession(ctx, agentName, threadID)
	if err != nil {
		return "", 0, fmt.Errorf("session setup: %w", err)
	}

	response, err := p.sendAndWait(ctx, session, prompt, agentName, threadID)
	if err != nil {
		return "", 0, err
	}

	response = p.extractAndSaveMemories(agentName, response)

	// Record the response message.
	msgID, err := p.store.AddMessage(store.Message{
		ThreadID:         threadID,
		FromName:         agentName,
		Content:          response,
		MessageType:      "agent_message",
		CopilotSessionID: session.SessionID,
		ParentMessageID:  parentMsgID,
	})
	if err != nil {
		log.Printf("failed to record response message: %v", err)
	} else if msg, err := p.store.GetMessage(msgID); err == nil && msg != nil {
		p.broadcast(*msg)
	}

	return response, msgID, nil
}

// extractAndSaveMemories parses [MEMORY ...] markers from a response,
// saves each valid memory, and returns the response with markers stripped.
func (p *Pool) extractAndSaveMemories(agentName, response string) string {
	matches := memoryMarkerRe.FindAllStringSubmatch(response, -1)
	if len(matches) == 0 {
		return response
	}
	for _, m := range matches {
		category := strings.TrimSpace(m[1])
		source := strings.TrimSpace(m[2])
		content := strings.TrimSpace(m[3])
		if content == "" {
			continue
		}
		if !store.ValidMemoryCategories[category] {
			log.Printf("agent %s used invalid memory category %q, skipping", agentName, category)
			continue
		}
		if _, err := p.SaveMemory(agentName, category, content, source); err != nil {
			log.Printf("failed to save memory for agent %s: %v", agentName, err)
		}
	}
	cleaned := memoryMarkerRe.ReplaceAllString(response, "")
	return strings.TrimSpace(cleaned)
}

// sendAndWait sends a prompt on the session and blocks until "session.idle".
// It logs intermediate session events to the session_events table.
func (p *Pool) sendAndWait(ctx context.Context, session *copilot.Session, text string, agentName string, threadID string) (string, error) {
	var (
		response string
		done     = make(chan struct{})
		once     sync.Once
	)

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch d := event.Data.(type) {
		case *copilot.AssistantMessageData:
			response = d.Content
			for _, tr := range d.ToolRequests {
				toolName := tr.Name
				if toolName == "" {
					toolName = "unknown"
				}
				_ = p.store.AddSessionEvent(store.SessionEvent{
					CopilotSessionID: session.SessionID,
					ThreadID:         threadID,
					AgentName:        agentName,
					EventType:        "tool_call",
					Content:          toolName,
					Metadata:         jsonMeta(map[string]string{"tool": toolName}),
				})
			}
		case *copilot.ToolExecutionStartData:
			_ = p.store.AddSessionEvent(store.SessionEvent{
				CopilotSessionID: session.SessionID,
				ThreadID:         threadID,
				AgentName:        agentName,
				EventType:        "tool_start",
				Content:          d.ToolName,
				Metadata:         jsonMeta(map[string]string{"tool": d.ToolName}),
			})
		case *copilot.ToolExecutionCompleteData:
			_ = p.store.AddSessionEvent(store.SessionEvent{
				CopilotSessionID: session.SessionID,
				ThreadID:         threadID,
				AgentName:        agentName,
				EventType:        "tool_complete",
				Content:          d.ToolCallID,
			})
		case *copilot.AssistantIntentData:
			_ = p.store.AddSessionEvent(store.SessionEvent{
				CopilotSessionID: session.SessionID,
				ThreadID:         threadID,
				AgentName:        agentName,
				EventType:        "intent",
				Content:          d.Intent,
			})
		case *copilot.AssistantReasoningData:
			text := d.Content
			if len(text) > 300 {
				text = text[:300] + "…"
			}
			if text != "" {
				_ = p.store.AddSessionEvent(store.SessionEvent{
					CopilotSessionID: session.SessionID,
					ThreadID:         threadID,
					AgentName:        agentName,
					EventType:        "reasoning",
					Content:          text,
				})
			}
		case *copilot.SessionIdleData:
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

// getOrCreateSession returns the cached session or creates/resumes one.
func (p *Pool) getOrCreateSession(ctx context.Context, agentName string, threadID string) (*copilot.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := sessionKey{agent: agentName, threadID: threadID}

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
	sessionID, err := p.store.GetThreadSessionID(threadID, agentName)
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
		log.Printf("failed to resume session %s for agent %s thread %s, creating new: %v", sessionID, agentName, threadID, err)
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

	// Inject context briefing (assigned tasks, projects).
	briefing, err := p.store.GetContextBriefing(agentName)
	if err != nil {
		log.Printf("failed to load context for agent %s: %v", agentName, err)
	} else if briefing != "" {
		systemPrompt += "\n\n" + briefing
	}

	// Inject thread context so tools can identify the current thread and agent.
	systemPrompt += fmt.Sprintf("\n\n[Session Context]\n"+
		"Your agent name is: %s\n"+
		"Current thread ID: %s\n\n"+
		"IMPORTANT: Before calling any company tool (bin/agent-msg, bin/agent-delegate, etc.), "+
		"you MUST export these environment variables:\n"+
		"  export THREAD_ID=%q AGENT_SELF=%q\n"+
		"This ensures your messages are tracked in the correct conversation thread.",
		agentName, threadID, threadID, agentName)

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

	if err := p.store.SaveThreadSession(threadID, agentName, session.SessionID); err != nil {
		log.Printf("failed to persist session for agent %s thread %s: %v", agentName, threadID, err)
	}

	p.sessions[key] = session
	return session, nil
}

// clearSession removes a session from the in-memory cache and destroys it.
func (p *Pool) clearSession(agentName string, threadID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := sessionKey{agent: agentName, threadID: threadID}
	if session, ok := p.sessions[key]; ok {
		_ = session.Destroy()
		delete(p.sessions, key)
	}
}

// SaveMemory stores a memory for an agent.
func (p *Pool) SaveMemory(agentName, category, content, source string) (int64, error) {
	return p.store.SaveMemory(store.Memory{
		AgentName: agentName,
		Category:  category,
		Content:   content,
		Source:    source,
	})
}

// CreateProject creates a new project on behalf of an agent.
func (p *Pool) CreateProject(agentName, id, name, description string) error {
	return p.store.CreateProject(store.Project{
		ID:          id,
		Name:        name,
		Description: description,
		Status:      "active",
		CreatedBy:   agentName,
	})
}

// CreateTask creates a new task in a project on behalf of an agent.
func (p *Pool) CreateTask(agentName, projectID, taskID, title, description string, priority int) error {
	return p.store.CreateTask(store.Task{
		ID:          taskID,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Status:      "backlog",
		CreatedBy:   agentName,
		Priority:    priority,
	})
}

// UpdateTaskStatus updates a task's status and adds a comment.
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
	return nil
}

// AssignTask assigns a task to an agent.
func (p *Pool) AssignTask(agentName, taskID, assignee string) error {
	t, err := p.store.GetTask(taskID)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	t.AssignedTo = &assignee
	return p.store.UpdateTask(*t)
}

// CommentOnTask adds a comment on a task.
func (p *Pool) CommentOnTask(agentName, taskID, comment string) error {
	_, err := p.store.AddTaskComment(store.TaskComment{
		TaskID:    taskID,
		AgentName: agentName,
		Content:   comment,
	})
	return err
}

// DelegateTask creates a task, assigns it, and runs the full engineering pipeline.
func (p *Pool) DelegateTask(ctx context.Context, fromAgent, toAgent string, chatID int64, projectID, taskID, title, instructions string, priority int, threadID string) error {
	if err := p.CreateTask(fromAgent, projectID, taskID, title, instructions, priority); err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	if err := p.AssignTask(fromAgent, taskID, toAgent); err != nil {
		return fmt.Errorf("assign task: %w", err)
	}
	if err := p.UpdateTaskStatus(fromAgent, taskID, "in_progress"); err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	reviewer := p.pickReviewer(toAgent)

	go p.runTaskPipeline(fromAgent, toAgent, reviewer, chatID, projectID, taskID, title, instructions, priority, threadID)

	return nil
}

// pickReviewer selects a review agent that is different from the worker.
func (p *Pool) pickReviewer(worker string) string {
	preferences := map[string][]string{
		"atlas":    {"sentinel", "pixel", "cto"},
		"pixel":    {"sentinel", "atlas", "cto"},
		"sentinel": {"atlas", "pixel", "cto"},
		"cto":      {"sentinel", "atlas", "pixel"},
	}
	if prefs, ok := preferences[worker]; ok {
		for _, candidate := range prefs {
			cfg, err := p.store.GetAgent(candidate)
			if err == nil && cfg != nil {
				return candidate
			}
		}
	}
	return "cto"
}

// runTaskPipeline executes the full implement → review → merge workflow.
func (p *Pool) runTaskPipeline(fromAgent, worker, reviewer string, chatID int64, projectID, taskID, title, instructions string, priority int, threadID string) {
	workCtx := context.Background()

	notify := func(msg string) {
		if p.notifyFunc != nil {
			p.notifyFunc(chatID, msg)
		}
	}

	fail := func(step string, err error) {
		log.Printf("task %s pipeline failed at %s: %v", taskID, step, err)
		_ = p.UpdateTaskStatus(fromAgent, taskID, "backlog")
		_ = p.CommentOnTask(fromAgent, taskID, fmt.Sprintf("pipeline failed at %s: %v", step, err))
		notify(fmt.Sprintf("❌ Task *%s* failed at %s: %v", title, step, err))
	}

	// Step 1: Worker implements the change.
	implementPrompt := fmt.Sprintf(
		"[Task Assignment: %s]\nTask ID: %s\nProject: %s\nPriority: %d\n\n"+
			"Instructions:\n%s\n\n"+
			"Follow the ENGINEERING.md workflow:\n"+
			"1. Create a worktree and branch\n"+
			"2. Implement the change\n"+
			"3. Run `go build ./...` and `go vet ./...`\n"+
			"4. Commit and push\n"+
			"5. Create a PR with `unset GITHUB_TOKEN && gh pr create`\n"+
			"6. Report the PR number in your response\n\n"+
			"Important: use `git` for standard git operations. Use the gh CLI (`unset GITHUB_TOKEN && gh ...`) only for GitHub API operations.",
		title, taskID, projectID, priority, instructions)

	implResponse, err := p.InvokeAgent(workCtx, threadID, fromAgent, worker, implementPrompt, nil)
	if err != nil {
		fail("implementation", err)
		return
	}
	_ = p.CommentOnTask(worker, taskID, "Implementation complete: "+implResponse)

	// Step 2: Move to review.
	_ = p.UpdateTaskStatus(worker, taskID, "review")

	reviewPrompt := fmt.Sprintf(
		"[Code Review Request from %s]\nTask: %s\nTask ID: %s\n\n"+
			"%s implemented this change. Please review:\n\n"+
			"Worker's summary:\n%s\n\n"+
			"Review the PR for correctness, security, and code quality. "+
			"If the changes look good, approve. If there are issues, describe them.\n\n"+
			"Use `unset GITHUB_TOKEN && gh pr list --state open` to find the PR, then review the diff with `unset GITHUB_TOKEN && gh pr diff <number>`.",
		worker, title, taskID, worker, implResponse)

	reviewResponse, err := p.InvokeAgent(workCtx, threadID, fromAgent, reviewer, reviewPrompt, nil)
	if err != nil {
		fail("review", err)
		return
	}
	_ = p.CommentOnTask(reviewer, taskID, "Review: "+reviewResponse)

	// Step 3: Merge.
	mergePrompt := fmt.Sprintf(
		"[Merge Request]\nTask: %s\nTask ID: %s\n\n"+
			"The reviewer (%s) has completed their review:\n%s\n\n"+
			"Please:\n"+
			"1. Check CI status with `unset GITHUB_TOKEN && gh pr checks <number>`\n"+
			"2. If CI passes, merge with `unset GITHUB_TOKEN && gh pr merge <number> --squash --delete-branch`\n"+
			"3. Clean up the worktree\n"+
			"4. Report whether the merge succeeded\n\n"+
			"If CI fails, fix the issues and try again.",
		title, taskID, reviewer, reviewResponse)

	mergeResponse, err := p.InvokeAgent(workCtx, threadID, fromAgent, worker, mergePrompt, nil)
	if err != nil {
		fail("merge", err)
		return
	}
	_ = p.CommentOnTask(worker, taskID, "Merge: "+mergeResponse)

	// Step 4: Mark done and notify.
	_ = p.UpdateTaskStatus(worker, taskID, "done")

	summary := mergeResponse
	if len(summary) > 500 {
		summary = summary[:500] + "…"
	}
	notify(fmt.Sprintf("✅ Task *%s* completed by %s (reviewed by %s):\n\n%s", title, worker, reviewer, summary))
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
			fmt.Fprintf(&sb, "  • %s → %s\n", t.Title, assignee)
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
			fmt.Fprintf(&sb, "  • %s → %s\n", t.Title, assignee)
		}
		sb.WriteString("\n")
	}

	if len(todo) > 0 {
		sb.WriteString("📋 *Up Next:*\n")
		for _, t := range todo {
			fmt.Fprintf(&sb, "  • %s\n", t.Title)
		}
	}

	return sb.String(), nil
}

// GetActiveThread returns the most recent active thread for a chat.
func (p *Pool) GetActiveThread(chatID int64) (*store.Thread, error) {
	return p.store.GetActiveThread(chatID)
}

// ListThreads returns threads matching the filter.
func (p *Pool) ListThreads(filter store.ThreadFilter) ([]store.Thread, error) {
	return p.store.ListThreads(filter)
}

// UpdateThreadStatus updates a thread's status.
func (p *Pool) UpdateThreadStatus(id, status string) error {
	return p.store.UpdateThreadStatus(id, status)
}

package agent

import (
	"context"
	"fmt"
	"log"
	"sync"

	copilot "github.com/github/copilot-sdk/go"

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
			return "", fmt.Errorf("send message: %w", err)
		}
	}

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
	session, err := p.client.CreateSession(ctx, &copilot.SessionConfig{
		Model:               cfg.Model,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		ConfigDir:           p.configDir,
		SystemMessage: &copilot.SystemMessageConfig{
			Content: cfg.SystemPrompt,
		},
	})
	if err != nil {
		return nil, err
	}

	if err := p.store.SaveSession(chatID, agentName, session.SessionID); err != nil {
		log.Printf("failed to persist session for agent %s chat %d: %v", agentName, chatID, err)
	}

	p.sessions[key] = session
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
	}
}

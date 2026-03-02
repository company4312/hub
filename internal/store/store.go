package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// AgentConfig holds the definition of a named agent.
type AgentConfig struct {
	Name         string
	Title        string
	SystemPrompt string
	Model        string
}

// Store manages agent definitions and chat-to-session mappings in a local SQLite database.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dbPath and ensures the schema exists.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS agents (
		name          TEXT PRIMARY KEY,
		title         TEXT NOT NULL,
		system_prompt TEXT NOT NULL,
		model         TEXT NOT NULL,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		chat_id    INTEGER NOT NULL,
		agent_name TEXT    NOT NULL DEFAULT 'cto',
		session_id TEXT    NOT NULL,
		created_at TEXT    NOT NULL,
		updated_at TEXT    NOT NULL,
		PRIMARY KEY (chat_id, agent_name)
	);

	CREATE TABLE IF NOT EXISTS chat_agents (
		chat_id    INTEGER PRIMARY KEY,
		agent_name TEXT    NOT NULL DEFAULT 'cto',
		updated_at TEXT    NOT NULL
	);
	`
	_, err := db.Exec(schema)
	return err
}

// RegisterAgent upserts an agent definition.
func (s *Store) RegisterAgent(cfg AgentConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO agents (name, title, system_prompt, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			title = excluded.title,
			system_prompt = excluded.system_prompt,
			model = excluded.model,
			updated_at = excluded.updated_at
	`, cfg.Name, cfg.Title, cfg.SystemPrompt, cfg.Model, now, now)
	return err
}

// GetAgent returns the config for a named agent, or nil if not found.
func (s *Store) GetAgent(name string) (*AgentConfig, error) {
	var cfg AgentConfig
	err := s.db.QueryRow(
		"SELECT name, title, system_prompt, model FROM agents WHERE name = ?", name,
	).Scan(&cfg.Name, &cfg.Title, &cfg.SystemPrompt, &cfg.Model)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListAgents returns all registered agent configs.
func (s *Store) ListAgents() ([]AgentConfig, error) {
	rows, err := s.db.Query("SELECT name, title, system_prompt, model FROM agents ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []AgentConfig
	for rows.Next() {
		var cfg AgentConfig
		if err := rows.Scan(&cfg.Name, &cfg.Title, &cfg.SystemPrompt, &cfg.Model); err != nil {
			return nil, err
		}
		agents = append(agents, cfg)
	}
	return agents, rows.Err()
}

// GetActiveAgent returns the active agent name for a chat, defaulting to "cto".
func (s *Store) GetActiveAgent(chatID int64) (string, error) {
	var name string
	err := s.db.QueryRow("SELECT agent_name FROM chat_agents WHERE chat_id = ?", chatID).Scan(&name)
	if err == sql.ErrNoRows {
		return "cto", nil
	}
	return name, err
}

// SetActiveAgent sets the active agent for a chat.
func (s *Store) SetActiveAgent(chatID int64, agentName string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO chat_agents (chat_id, agent_name, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET agent_name = excluded.agent_name, updated_at = excluded.updated_at
	`, chatID, agentName, now)
	return err
}

// GetSessionID returns the Copilot session ID for a chat and agent, or "" if none exists.
func (s *Store) GetSessionID(chatID int64, agentName string) (string, error) {
	var sessionID string
	err := s.db.QueryRow(
		"SELECT session_id FROM sessions WHERE chat_id = ? AND agent_name = ?", chatID, agentName,
	).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sessionID, err
}

// SaveSession upserts a session mapping for a chat and agent.
func (s *Store) SaveSession(chatID int64, agentName string, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO sessions (chat_id, agent_name, session_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, agent_name) DO UPDATE SET session_id = excluded.session_id, updated_at = excluded.updated_at
	`, chatID, agentName, sessionID, now, now)
	return err
}

// DeleteSession removes the session mapping for a chat and agent.
func (s *Store) DeleteSession(chatID int64, agentName string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE chat_id = ? AND agent_name = ?", chatID, agentName)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

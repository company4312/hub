package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// AgentConfig holds the definition of a named agent.
type AgentConfig struct {
	Name         string `yaml:"name"`
	Title        string `yaml:"title"`
	SystemPrompt string `yaml:"system_prompt"`
	Model        string `yaml:"model"`
}

// ActivityEntry represents a single activity log row.
type ActivityEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	AgentName string `json:"agent_name"`
	EventType string `json:"event_type"`
	Content   string `json:"content"`
	Metadata  string `json:"metadata,omitempty"`
	ChatID    int64  `json:"chat_id"`
}

// ActivityFilter controls which activity entries are returned.
type ActivityFilter struct {
	AgentName string
	EventType string
	Limit     int
	Since     string // RFC3339 timestamp
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
		_ = db.Close()
		return nil, err
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	// Create the migrations tracking table.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT    NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		if err := db.QueryRow(
			"SELECT 1 FROM schema_migrations WHERE version = ?", m.version,
		).Scan(&exists); err == nil {
			continue // already applied
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		if err := m.run(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", m.version, now,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

type migration struct {
	version int
	name    string
	run     func(tx *sql.Tx) error
}

// migrations is the ordered list of all schema migrations.
// Append new migrations here; never modify or reorder existing entries.
var migrations = []migration{
	{
		version: 1,
		name:    "create initial tables",
		run: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS sessions (
					chat_id    INTEGER PRIMARY KEY,
					session_id TEXT    NOT NULL,
					created_at TEXT    NOT NULL,
					updated_at TEXT    NOT NULL
				)
			`)
			return err
		},
	},
	{
		version: 2,
		name:    "add agents and multi-agent sessions",
		run: func(tx *sql.Tx) error {
			// Create agents table.
			if _, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS agents (
					name          TEXT PRIMARY KEY,
					title         TEXT NOT NULL,
					system_prompt TEXT NOT NULL,
					model         TEXT NOT NULL,
					created_at    TEXT NOT NULL,
					updated_at    TEXT NOT NULL
				)
			`); err != nil {
				return err
			}

			// Recreate sessions with composite PK (chat_id, agent_name).
			// Check if the old schema exists (no agent_name column).
			hasAgentName := false
			rows, err := tx.Query("PRAGMA table_info(sessions)")
			if err != nil {
				return err
			}
			for rows.Next() {
				var cid int
				var name, typ string
				var notNull, pk int
				var dflt sql.NullString
				if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
					_ = rows.Close()
					return err
				}
				if name == "agent_name" {
					hasAgentName = true
				}
			}
			_ = rows.Close()

			if !hasAgentName {
				if _, err := tx.Exec(`
					ALTER TABLE sessions RENAME TO sessions_old;

					CREATE TABLE sessions (
						chat_id    INTEGER NOT NULL,
						agent_name TEXT    NOT NULL DEFAULT 'cto',
						session_id TEXT    NOT NULL,
						created_at TEXT    NOT NULL,
						updated_at TEXT    NOT NULL,
						PRIMARY KEY (chat_id, agent_name)
					);

					INSERT INTO sessions (chat_id, agent_name, session_id, created_at, updated_at)
					SELECT chat_id, 'cto', session_id, created_at, updated_at FROM sessions_old;

					DROP TABLE sessions_old;
				`); err != nil {
					return err
				}
			}

			// Create chat_agents table.
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS chat_agents (
					chat_id    INTEGER PRIMARY KEY,
					agent_name TEXT    NOT NULL DEFAULT 'cto',
					updated_at TEXT    NOT NULL
				)
			`)
			return err
		},
	},
	{
		version: 3,
		name:    "create activity log table",
		run: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`
				CREATE TABLE activity_log (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					timestamp  TEXT    NOT NULL,
					agent_name TEXT    NOT NULL,
					event_type TEXT    NOT NULL,
					content    TEXT    NOT NULL,
					metadata   TEXT,
					chat_id    INTEGER NOT NULL DEFAULT 0
				)
			`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX idx_activity_log_agent ON activity_log(agent_name)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX idx_activity_log_type ON activity_log(event_type)`); err != nil {
				return err
			}
			_, err := tx.Exec(`CREATE INDEX idx_activity_log_time ON activity_log(timestamp DESC)`)
			return err
		},
	},
}

// LogActivity inserts an activity log entry.
func (s *Store) LogActivity(entry ActivityEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO activity_log (timestamp, agent_name, event_type, content, metadata, chat_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, entry.Timestamp, entry.AgentName, entry.EventType, entry.Content, entry.Metadata, entry.ChatID)
	return err
}

// GetActivities returns activity entries matching the given filter.
func (s *Store) GetActivities(filter ActivityFilter) ([]ActivityEntry, error) {
	query := "SELECT id, timestamp, agent_name, event_type, content, COALESCE(metadata,''), chat_id FROM activity_log WHERE 1=1"
	var args []any

	if filter.AgentName != "" {
		query += " AND agent_name = ?"
		args = append(args, filter.AgentName)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.Since != "" {
		query += " AND timestamp >= ?"
		args = append(args, filter.Since)
	}

	query += " ORDER BY timestamp DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.AgentName, &e.EventType, &e.Content, &e.Metadata, &e.ChatID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
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
	defer func() { _ = rows.Close() }()

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

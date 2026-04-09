package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/domuk-k/open-managed-agents/internal/session"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/001_init.up.sql
var migrationSQL string

//go:embed migrations/002_messages.up.sql
var migration002SQL string

// SQLiteStore implements the Store interface backed by SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens a SQLite database at dbPath, runs migrations, and returns a Store.
func NewSQLiteStore(dbPath string) (Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(migrationSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if _, err := db.Exec(migration002SQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migration 002: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// agentConfig is the JSON blob stored in the config column (everything except id, version, timestamps).
type agentConfig struct {
	Name           string                  `json:"name"`
	Model          agent.ModelConfig       `json:"model"`
	System         *string                 `json:"system,omitempty"`
	Tools          []agent.ToolConfig      `json:"tools,omitempty"`
	McpServers     []agent.McpServerConfig `json:"mcp_servers,omitempty"`
	Skills         []agent.SkillConfig     `json:"skills,omitempty"`
	CallableAgents []string                `json:"callable_agents,omitempty"`
	Description    *string                 `json:"description,omitempty"`
	Metadata       map[string]string       `json:"metadata,omitempty"`
}

func marshalAgentConfig(a *agent.Agent) ([]byte, error) {
	cfg := agentConfig{
		Name:           a.Name,
		Model:          a.Model,
		System:         a.System,
		Tools:          a.Tools,
		McpServers:     a.McpServers,
		Skills:         a.Skills,
		CallableAgents: a.CallableAgents,
		Description:    a.Description,
		Metadata:       a.Metadata,
	}
	return json.Marshal(cfg)
}

func unmarshalAgentConfig(data []byte, a *agent.Agent) error {
	var cfg agentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	a.Name = cfg.Name
	a.Model = cfg.Model
	a.System = cfg.System
	a.Tools = cfg.Tools
	a.McpServers = cfg.McpServers
	a.Skills = cfg.Skills
	a.CallableAgents = cfg.CallableAgents
	a.Description = cfg.Description
	a.Metadata = cfg.Metadata
	return nil
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func parseOptionalTime(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

// --- Agents ---

func (s *SQLiteStore) CreateAgent(ctx context.Context, a *agent.Agent) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	ts := now()
	a.Version = 1
	a.CreatedAt = parseTime(ts)
	a.UpdatedAt = a.CreatedAt

	configJSON, err := marshalAgentConfig(a)
	if err != nil {
		return fmt.Errorf("marshal agent config: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, config, version, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
		a.ID, a.Name, string(configJSON), ts, ts,
	)
	return err
}

func (s *SQLiteStore) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, config, version, created_at, updated_at, archived_at FROM agents WHERE id = ? AND archived_at IS NULL`, id)

	var a agent.Agent
	var configStr string
	var createdAt, updatedAt string
	var archivedAt sql.NullString

	if err := row.Scan(&a.ID, &a.Name, &configStr, &a.Version, &createdAt, &updatedAt, &archivedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("agent not found: %s", id)
		}
		return nil, err
	}

	if err := unmarshalAgentConfig([]byte(configStr), &a); err != nil {
		return nil, fmt.Errorf("unmarshal agent config: %w", err)
	}
	a.CreatedAt = parseTime(createdAt)
	a.UpdatedAt = parseTime(updatedAt)
	a.ArchivedAt = parseOptionalTime(archivedAt)
	return &a, nil
}

func (s *SQLiteStore) ListAgents(ctx context.Context) ([]*agent.Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, config, version, created_at, updated_at, archived_at FROM agents WHERE archived_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]*agent.Agent, 0)
	for rows.Next() {
		var a agent.Agent
		var configStr string
		var createdAt, updatedAt string
		var archivedAt sql.NullString

		if err := rows.Scan(&a.ID, &a.Name, &configStr, &a.Version, &createdAt, &updatedAt, &archivedAt); err != nil {
			return nil, err
		}
		if err := unmarshalAgentConfig([]byte(configStr), &a); err != nil {
			return nil, fmt.Errorf("unmarshal agent config: %w", err)
		}
		a.CreatedAt = parseTime(createdAt)
		a.UpdatedAt = parseTime(updatedAt)
		a.ArchivedAt = parseOptionalTime(archivedAt)
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

func (s *SQLiteStore) UpdateAgent(ctx context.Context, a *agent.Agent, expectedVersion int) error {
	ts := now()
	configJSON, err := marshalAgentConfig(a)
	if err != nil {
		return fmt.Errorf("marshal agent config: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET name = ?, config = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ? AND archived_at IS NULL`,
		a.Name, string(configJSON), ts, a.ID, expectedVersion,
	)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("optimistic lock failed: agent %s version mismatch (expected %d)", a.ID, expectedVersion)
	}

	a.Version = expectedVersion + 1
	a.UpdatedAt = parseTime(ts)
	return nil
}

func (s *SQLiteStore) ArchiveAgent(ctx context.Context, id string) error {
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`,
		ts, ts, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("agent not found or already archived: %s", id)
	}
	return nil
}

func (s *SQLiteStore) CreateAgentVersion(ctx context.Context, agentID string, version int, config []byte) error {
	ts := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_versions (agent_id, version, config, created_at) VALUES (?, ?, ?, ?)`,
		agentID, version, string(config), ts,
	)
	return err
}

func (s *SQLiteStore) GetAgentVersions(ctx context.Context, agentID string) ([]AgentVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, version, config, created_at FROM agent_versions WHERE agent_id = ? ORDER BY version DESC`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]AgentVersion, 0)
	for rows.Next() {
		var v AgentVersion
		if err := rows.Scan(&v.AgentID, &v.Version, &v.Config, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// --- Environments ---

func (s *SQLiteStore) CreateEnvironment(ctx context.Context, e *environment.Environment) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	ts := now()
	e.CreatedAt = parseTime(ts)
	e.UpdatedAt = e.CreatedAt

	configJSON, err := json.Marshal(e.Config)
	if err != nil {
		return fmt.Errorf("marshal environment config: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO environments (id, name, config, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.Name, string(configJSON), ts, ts,
	)
	return err
}

func (s *SQLiteStore) GetEnvironment(ctx context.Context, id string) (*environment.Environment, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, config, created_at, updated_at, archived_at FROM environments WHERE id = ? AND archived_at IS NULL`, id)

	var e environment.Environment
	var configStr string
	var createdAt, updatedAt string
	var archivedAt sql.NullString

	if err := row.Scan(&e.ID, &e.Name, &configStr, &createdAt, &updatedAt, &archivedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("environment not found: %s", id)
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(configStr), &e.Config); err != nil {
		return nil, fmt.Errorf("unmarshal environment config: %w", err)
	}
	e.CreatedAt = parseTime(createdAt)
	e.UpdatedAt = parseTime(updatedAt)
	e.ArchivedAt = parseOptionalTime(archivedAt)
	return &e, nil
}

func (s *SQLiteStore) ListEnvironments(ctx context.Context) ([]*environment.Environment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, config, created_at, updated_at, archived_at FROM environments WHERE archived_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	envs := make([]*environment.Environment, 0)
	for rows.Next() {
		var e environment.Environment
		var configStr string
		var createdAt, updatedAt string
		var archivedAt sql.NullString

		if err := rows.Scan(&e.ID, &e.Name, &configStr, &createdAt, &updatedAt, &archivedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(configStr), &e.Config); err != nil {
			return nil, fmt.Errorf("unmarshal environment config: %w", err)
		}
		e.CreatedAt = parseTime(createdAt)
		e.UpdatedAt = parseTime(updatedAt)
		e.ArchivedAt = parseOptionalTime(archivedAt)
		envs = append(envs, &e)
	}
	return envs, rows.Err()
}

func (s *SQLiteStore) ArchiveEnvironment(ctx context.Context, id string) error {
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE environments SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`,
		ts, ts, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("environment not found or already archived: %s", id)
	}
	return nil
}

// --- Sessions ---

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *session.Session) error {
	if sess.ID == "" {
		sess.ID = uuid.New().String()
	}
	ts := now()
	sess.CreatedAt = parseTime(ts)
	sess.UpdatedAt = sess.CreatedAt
	if sess.Status == "" {
		sess.Status = session.StatusStarting
	}

	var metadataStr *string
	if sess.Metadata != nil {
		b, err := json.Marshal(sess.Metadata)
		if err != nil {
			return fmt.Errorf("marshal session metadata: %w", err)
		}
		str := string(b)
		metadataStr = &str
	}

	var title *string
	if sess.Title != nil {
		title = sess.Title
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, agent_id, agent_version, environment_id, title, status, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Agent, sess.AgentVersion, sess.EnvironmentID, title, string(sess.Status), metadataStr, ts, ts,
	)
	return err
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*session.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, agent_version, environment_id, title, status, metadata, created_at, updated_at, completed_at FROM sessions WHERE id = ?`, id)

	var sess session.Session
	var title sql.NullString
	var status string
	var metadataStr sql.NullString
	var createdAt, updatedAt string
	var completedAt sql.NullString

	if err := row.Scan(&sess.ID, &sess.Agent, &sess.AgentVersion, &sess.EnvironmentID, &title, &status, &metadataStr, &createdAt, &updatedAt, &completedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, err
	}

	sess.Status = session.SessionStatus(status)
	if title.Valid {
		sess.Title = &title.String
	}
	if metadataStr.Valid {
		if err := json.Unmarshal([]byte(metadataStr.String), &sess.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal session metadata: %w", err)
		}
	}
	sess.CreatedAt = parseTime(createdAt)
	sess.UpdatedAt = parseTime(updatedAt)
	sess.CompletedAt = parseOptionalTime(completedAt)
	return &sess, nil
}

func (s *SQLiteStore) ListSessions(ctx context.Context) ([]*session.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, agent_version, environment_id, title, status, metadata, created_at, updated_at, completed_at FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]*session.Session, 0)
	for rows.Next() {
		var sess session.Session
		var title sql.NullString
		var status string
		var metadataStr sql.NullString
		var createdAt, updatedAt string
		var completedAt sql.NullString

		if err := rows.Scan(&sess.ID, &sess.Agent, &sess.AgentVersion, &sess.EnvironmentID, &title, &status, &metadataStr, &createdAt, &updatedAt, &completedAt); err != nil {
			return nil, err
		}
		sess.Status = session.SessionStatus(status)
		if title.Valid {
			sess.Title = &title.String
		}
		if metadataStr.Valid {
			if err := json.Unmarshal([]byte(metadataStr.String), &sess.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal session metadata: %w", err)
			}
		}
		sess.CreatedAt = parseTime(createdAt)
		sess.UpdatedAt = parseTime(updatedAt)
		sess.CompletedAt = parseOptionalTime(completedAt)
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

func (s *SQLiteStore) UpdateSessionStatus(ctx context.Context, id string, status session.SessionStatus) error {
	ts := now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), ts, id,
	)
	return err
}

// --- Messages ---

func (s *SQLiteStore) SaveMessages(ctx context.Context, sessionID string, messages []byte) error {
	ts := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_messages (session_id, messages, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET messages = excluded.messages, updated_at = excluded.updated_at`,
		sessionID, string(messages), ts,
	)
	return err
}

func (s *SQLiteStore) GetMessages(ctx context.Context, sessionID string) ([]byte, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT messages FROM session_messages WHERE session_id = ?`, sessionID)

	var messagesStr string
	if err := row.Scan(&messagesStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no messages found for session: %s", sessionID)
		}
		return nil, err
	}
	return []byte(messagesStr), nil
}

// --- Events ---

func (s *SQLiteStore) InsertEvent(ctx context.Context, e *StoredEvent) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = now()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, session_id, type, data, created_at) VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.SessionID, e.Type, string(e.Data), e.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetSessionEvents(ctx context.Context, sessionID string) ([]*StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, type, data, created_at FROM events WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]*StoredEvent, 0)
	for rows.Next() {
		var e StoredEvent
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Type, &e.Data, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

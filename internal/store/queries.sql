-- name: CreateAgent :one
INSERT INTO agents (id, name, config, version, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: GetAgent :one
SELECT * FROM agents WHERE id = ? AND archived_at IS NULL;

-- name: ListAgents :many
SELECT * FROM agents WHERE archived_at IS NULL ORDER BY created_at DESC;

-- name: UpdateAgent :one
UPDATE agents SET config = ?, version = version + 1, updated_at = ?
WHERE id = ? AND version = ? AND archived_at IS NULL
RETURNING *;

-- name: ArchiveAgent :one
UPDATE agents SET archived_at = ?, updated_at = ?
WHERE id = ? AND archived_at IS NULL
RETURNING *;

-- name: CreateAgentVersion :exec
INSERT INTO agent_versions (agent_id, version, config, created_at)
VALUES (?, ?, ?, ?);

-- name: GetAgentVersions :many
SELECT * FROM agent_versions WHERE agent_id = ? ORDER BY version DESC;

-- name: CreateEnvironment :one
INSERT INTO environments (id, name, config, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetEnvironment :one
SELECT * FROM environments WHERE id = ? AND archived_at IS NULL;

-- name: ListEnvironments :many
SELECT * FROM environments WHERE archived_at IS NULL ORDER BY created_at DESC;

-- name: ArchiveEnvironment :one
UPDATE environments SET archived_at = ?, updated_at = ?
WHERE id = ? AND archived_at IS NULL
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (id, agent_id, agent_version, environment_id, title, status, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: ListSessions :many
SELECT * FROM sessions ORDER BY created_at DESC;

-- name: UpdateSessionStatus :exec
UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?;

-- name: InsertEvent :exec
INSERT INTO events (id, session_id, type, data, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionEvents :many
SELECT * FROM events WHERE session_id = ? ORDER BY created_at ASC;

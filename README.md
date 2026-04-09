# open-managed-agents (OMA)

**Self-hosted, provider-agnostic managed agents platform. Single binary.**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Build Status](https://img.shields.io/github/actions/workflow/status/domuk-k/open-managed-agents/ci.yml?branch=main)](https://github.com/domuk-k/open-managed-agents/actions)

---

## What is this?

OMA is an open-source alternative to Claude Managed Agents. It gives you a self-hosted platform to create, manage, and orchestrate AI agents with sandboxed execution environments.

- **Provider-agnostic** -- works with any OpenAI-compatible LLM API (LM Studio, Ollama, vLLM, OpenAI, Anthropic via proxy, etc.)
- **Single binary** -- one Go binary, no runtime dependencies beyond SQLite
- **Docker sandboxing** -- isolate agent execution in containers, or run locally
- **REST API** -- full CRUD for agents, environments, and sessions with SSE streaming
- **Built-in tools** -- bash execution, file read/write/edit, glob, grep

## Quick Start

### Docker (recommended)

```bash
docker compose up -d
```

The server starts on `http://localhost:8080`. Configure your LLM endpoint via environment variables in `docker-compose.yml`.

### Binary

Download the latest release:

```bash
curl -fsSL https://github.com/domuk-k/open-managed-agents/releases/latest/download/install.sh | bash
oma server start
```

### Go Install

```bash
go install github.com/domuk-k/open-managed-agents/cmd/oma@latest
oma server start
```

### npm

```bash
npm install -g @oma/cli
oma server start
```

## Usage

```bash
# Start the server
oma server start --port 8080

# Create an agent
oma agents create --name "Coder" --model "openai/gpt-4o" --system "You are a coder"

# Create an environment
oma env create --name "sandbox" --type docker

# Create a session (binds agent + environment)
oma sessions create --agent <agent-id> --env <environment-id>

# Stream session events (SSE)
oma sessions stream <session-id>
```

## REST API

All endpoints are under the `/v1` prefix.

### Agents

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/agents` | Create a new agent |
| `GET` | `/v1/agents` | List all agents |
| `GET` | `/v1/agents/:id` | Get agent by ID |
| `POST` | `/v1/agents/:id` | Update agent (optimistic locking) |
| `POST` | `/v1/agents/:id/archive` | Archive an agent |
| `GET` | `/v1/agents/:id/versions` | List agent version history |

### Environments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/environments` | Create an environment |
| `GET` | `/v1/environments` | List all environments |
| `GET` | `/v1/environments/:id` | Get environment by ID |
| `POST` | `/v1/environments/:id/archive` | Archive an environment |

### Sessions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sessions` | Create a session |
| `GET` | `/v1/sessions` | List all sessions |
| `GET` | `/v1/sessions/:id` | Get session by ID |
| `POST` | `/v1/sessions/:id/events` | Post an event (user message) |
| `GET` | `/v1/sessions/:id/stream` | Stream events via SSE |
| `GET` | `/v1/sessions/:id/events` | Get all session events |
| `POST` | `/v1/sessions/:id/pause` | Pause a running session |
| `POST` | `/v1/sessions/:id/resume` | Resume a paused session |
| `GET` | `/v1/sessions/:id/evaluation` | Get session evaluation/outcome |

### Example: Create an Agent

```bash
curl -X POST http://localhost:8080/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coder",
    "model": "openai/gpt-4o",
    "system_prompt": "You are a helpful coding assistant."
  }'
```

## Configuration

All configuration is done via environment variables with the `OMA_` prefix.

| Variable | Description | Default |
|----------|-------------|---------|
| `OMA_PORT` | HTTP server port | `8080` |
| `OMA_DB_PATH` | SQLite database path | `./data/oma.db` |
| `OMA_SANDBOX_TYPE` | Sandbox backend (`local` or `docker`) | `local` |
| `OMA_LLM_BASE_URL` | LLM API base URL (OpenAI-compatible) | `http://localhost:1234/v1` |
| `OMA_LLM_MODEL` | Default model identifier | `qwen3.5-35b-a3b` |
| `OMA_LLM_API_KEY` | API key for the LLM provider | _(empty)_ |
| `OMA_API_KEY` | API key to protect the OMA server | _(empty)_ |

## Architecture

```
cmd/oma/          CLI entrypoint (cobra)
cli/              CLI command definitions (server, agents, environments, sessions)
internal/
  api/            Echo HTTP server, REST handlers, SSE streaming
  agent/          Agent domain types
  environment/    Environment domain types
  session/        Session engine, EventBus (pub/sub)
  llm/            LLM provider interface + OpenAI-compatible client
  sandbox/        Sandbox interface (Docker / Local backends)
  tools/          Built-in tools (bash, file_read, file_write, file_edit, glob, grep)
  mcp/            MCP (Model Context Protocol) client integration
  store/          Store interface + SQLite implementation (sqlc)
  config/         Environment variable loading
```

## Development

```bash
# Build binary to bin/oma
make build

# Run server in dev mode (go run)
make dev

# Run all tests
make test

# Lint
make lint

# Run with Docker
make docker

# Clean build artifacts
make clean
```

### Prerequisites

- Go 1.26+ (must match go.mod)
- CGO enabled (required for SQLite)
- Docker (optional, for container sandboxing)

> **Note:** Windows is not supported. OMA targets Linux and macOS only.

## License

[MIT](LICENSE)

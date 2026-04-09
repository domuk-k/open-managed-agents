package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/config"
	"github.com/domuk-k/open-managed-agents/internal/store"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		Port:   "8080",
		DBPath: ":memory:",
	}
	return NewServer(cfg, s)
}

func TestCreateAgent(t *testing.T) {
	srv := setupTestServer(t)

	body := agent.CreateRequest{
		Name:  "test-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", created.Name)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Version != 1 {
		t.Errorf("expected version 1, got %d", created.Version)
	}
}

func TestCreateAgentValidation(t *testing.T) {
	srv := setupTestServer(t)

	// Missing name
	body := agent.CreateRequest{Model: agent.ModelConfig{ID: "gpt-4"}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Missing model.id
	body = agent.CreateRequest{Name: "test"}
	b, _ = json.Marshal(body)
	req = httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetAgent(t *testing.T) {
	srv := setupTestServer(t)

	// Create an agent first
	body := agent.CreateRequest{
		Name:  "test-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var created agent.Agent
	json.NewDecoder(rec.Body).Decode(&created)

	// Get the agent
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/"+created.ID, nil)
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var fetched agent.Agent
	json.NewDecoder(rec.Body).Decode(&fetched)

	if fetched.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, fetched.ID)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListAgents(t *testing.T) {
	srv := setupTestServer(t)

	// Create two agents
	for _, name := range []string{"agent-1", "agent-2"} {
		body := agent.CreateRequest{
			Name:  name,
			Model: agent.ModelConfig{ID: "gpt-4"},
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rec.Code)
		}
	}

	// List agents
	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var agents []*agent.Agent
	json.NewDecoder(rec.Body).Decode(&agents)

	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestUpdateAgent(t *testing.T) {
	srv := setupTestServer(t)

	// Create agent
	createBody := agent.CreateRequest{
		Name:  "original",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var created agent.Agent
	json.NewDecoder(rec.Body).Decode(&created)

	// Update agent
	newName := "updated"
	updateBody := agent.UpdateRequest{
		Version: 1,
		Name:    &newName,
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+created.ID, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated agent.Agent
	json.NewDecoder(rec.Body).Decode(&updated)

	if updated.Name != "updated" {
		t.Errorf("expected name 'updated', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Errorf("expected version 2, got %d", updated.Version)
	}
}

func TestUpdateAgentConflict(t *testing.T) {
	srv := setupTestServer(t)

	// Create agent
	createBody := agent.CreateRequest{
		Name:  "original",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var created agent.Agent
	json.NewDecoder(rec.Body).Decode(&created)

	// Update with wrong version
	newName := "updated"
	updateBody := agent.UpdateRequest{
		Version: 99,
		Name:    &newName,
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+created.ID, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestArchiveAgent(t *testing.T) {
	srv := setupTestServer(t)

	// Create agent
	createBody := agent.CreateRequest{
		Name:  "to-archive",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var created agent.Agent
	json.NewDecoder(rec.Body).Decode(&created)

	// Archive
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+created.ID+"/archive", nil)
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should no longer appear in list
	req = httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var agents []*agent.Agent
	json.NewDecoder(rec.Body).Decode(&agents)

	if len(agents) != 0 {
		t.Errorf("expected 0 agents after archive, got %d", len(agents))
	}
}

func TestGetAgentVersions(t *testing.T) {
	srv := setupTestServer(t)

	// Create agent
	createBody := agent.CreateRequest{
		Name:  "versioned",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	var created agent.Agent
	json.NewDecoder(rec.Body).Decode(&created)

	// Update to create version 2
	newName := "versioned-v2"
	updateBody := agent.UpdateRequest{Version: 1, Name: &newName}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+created.ID, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	// Get versions
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/"+created.ID+"/versions", nil)
	rec = httptest.NewRecorder()
	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var versions []store.AgentVersion
	json.NewDecoder(rec.Body).Decode(&versions)

	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

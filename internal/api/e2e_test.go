package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/config"
	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/domuk-k/open-managed-agents/internal/session"
	"github.com/domuk-k/open-managed-agents/internal/store"
)

// setupE2EServer creates a Server backed by a real SQLite DB in t.TempDir().
func setupE2EServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	dbPath := t.TempDir() + "/e2e_test.db"
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		Port:   "0",
		DBPath: dbPath,
	}
	srv := NewServer(cfg, s)

	ts := httptest.NewServer(srv.echo)
	t.Cleanup(func() { ts.Close() })
	return srv, ts
}

// jsonPost is a helper that POSTs JSON to the given URL and returns the response.
func jsonPost(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// jsonGet is a helper that GETs the given URL and returns the response.
func jsonGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// decodeJSON decodes a response body into the target and closes the body.
func decodeJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// assertStatus checks the response status code.
func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", expected, resp.StatusCode, string(body))
	}
}

// ---------------------------------------------------------------------------
// Test: Full Agent CRUD flow
// ---------------------------------------------------------------------------

func TestE2E_AgentCRUDFlow(t *testing.T) {
	_, ts := setupE2EServer(t)

	// 1. POST /v1/agents -> create agent -> verify 201 + has ID
	createReq := agent.CreateRequest{
		Name:  "e2e-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	resp := jsonPost(t, ts.URL+"/v1/agents", createReq)
	assertStatus(t, resp, http.StatusCreated)

	var created agent.Agent
	decodeJSON(t, resp, &created)
	if created.ID == "" {
		t.Fatal("expected non-empty agent ID")
	}
	if created.Name != "e2e-agent" {
		t.Fatalf("expected name 'e2e-agent', got %q", created.Name)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}

	agentID := created.ID

	// 2. GET /v1/agents -> list -> contains our agent
	resp = jsonGet(t, ts.URL+"/v1/agents")
	assertStatus(t, resp, http.StatusOK)

	var agents []*agent.Agent
	decodeJSON(t, resp, &agents)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent in list, got %d", len(agents))
	}
	if agents[0].ID != agentID {
		t.Fatalf("expected agent ID %q, got %q", agentID, agents[0].ID)
	}

	// 3. GET /v1/agents/:id -> get -> verify matches
	resp = jsonGet(t, ts.URL+"/v1/agents/"+agentID)
	assertStatus(t, resp, http.StatusOK)

	var fetched agent.Agent
	decodeJSON(t, resp, &fetched)
	if fetched.ID != agentID {
		t.Fatalf("expected agent ID %q, got %q", agentID, fetched.ID)
	}
	if fetched.Name != "e2e-agent" {
		t.Fatalf("expected name 'e2e-agent', got %q", fetched.Name)
	}

	// 4. POST /v1/agents/:id -> update with version=1 -> verify 200 + version=2
	newName := "e2e-agent-updated"
	updateReq := agent.UpdateRequest{
		Version: 1,
		Name:    &newName,
	}
	resp = jsonPost(t, ts.URL+"/v1/agents/"+agentID, updateReq)
	assertStatus(t, resp, http.StatusOK)

	var updated agent.Agent
	decodeJSON(t, resp, &updated)
	if updated.Name != "e2e-agent-updated" {
		t.Fatalf("expected name 'e2e-agent-updated', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	// 5. POST /v1/agents/:id -> update with stale version=1 -> verify 409
	staleName := "should-fail"
	staleReq := agent.UpdateRequest{
		Version: 1,
		Name:    &staleName,
	}
	resp = jsonPost(t, ts.URL+"/v1/agents/"+agentID, staleReq)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()

	// 6. GET /v1/agents/:id/versions -> verify 2 versions
	resp = jsonGet(t, ts.URL+"/v1/agents/"+agentID+"/versions")
	assertStatus(t, resp, http.StatusOK)

	var versions []store.AgentVersion
	decodeJSON(t, resp, &versions)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// 7. POST /v1/agents/:id/archive -> verify 200
	resp = jsonPost(t, ts.URL+"/v1/agents/"+agentID+"/archive", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 8. GET /v1/agents/:id -> verify 404 (archived)
	resp = jsonGet(t, ts.URL+"/v1/agents/"+agentID)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Test: Environment CRUD flow
// ---------------------------------------------------------------------------

func TestE2E_EnvironmentCRUDFlow(t *testing.T) {
	_, ts := setupE2EServer(t)

	// 1. POST /v1/environments -> create -> 201
	createReq := environment.CreateRequest{
		Name: "e2e-env",
		Config: environment.EnvironmentConfig{
			Type:       "local",
			Networking: environment.NetworkConfig{Type: "full"},
		},
	}
	resp := jsonPost(t, ts.URL+"/v1/environments", createReq)
	assertStatus(t, resp, http.StatusCreated)

	var created environment.Environment
	decodeJSON(t, resp, &created)
	if created.ID == "" {
		t.Fatal("expected non-empty environment ID")
	}
	if created.Name != "e2e-env" {
		t.Fatalf("expected name 'e2e-env', got %q", created.Name)
	}

	envID := created.ID

	// 2. GET /v1/environments -> list -> contains it
	resp = jsonGet(t, ts.URL+"/v1/environments")
	assertStatus(t, resp, http.StatusOK)

	var envs []*environment.Environment
	decodeJSON(t, resp, &envs)
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].ID != envID {
		t.Fatalf("expected env ID %q, got %q", envID, envs[0].ID)
	}

	// 3. POST /v1/environments/:id/archive -> 200
	resp = jsonPost(t, ts.URL+"/v1/environments/"+envID+"/archive", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Verify archived: list should be empty
	resp = jsonGet(t, ts.URL+"/v1/environments")
	assertStatus(t, resp, http.StatusOK)

	var envsAfter []*environment.Environment
	decodeJSON(t, resp, &envsAfter)
	if len(envsAfter) != 0 {
		t.Fatalf("expected 0 environments after archive, got %d", len(envsAfter))
	}
}

// ---------------------------------------------------------------------------
// Test: Session lifecycle
// ---------------------------------------------------------------------------

func TestE2E_SessionLifecycle(t *testing.T) {
	_, ts := setupE2EServer(t)

	// Create agent first
	agentReq := agent.CreateRequest{
		Name:  "session-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	resp := jsonPost(t, ts.URL+"/v1/agents", agentReq)
	assertStatus(t, resp, http.StatusCreated)
	var ag agent.Agent
	decodeJSON(t, resp, &ag)

	// Create environment first
	envReq := environment.CreateRequest{
		Name: "session-env",
		Config: environment.EnvironmentConfig{
			Type:       "local",
			Networking: environment.NetworkConfig{Type: "full"},
		},
	}
	resp = jsonPost(t, ts.URL+"/v1/environments", envReq)
	assertStatus(t, resp, http.StatusCreated)
	var env environment.Environment
	decodeJSON(t, resp, &env)

	// 1. POST /v1/sessions -> create session -> 201 + status=starting
	sessReq := session.CreateRequest{
		AgentID:       ag.ID,
		EnvironmentID: env.ID,
	}
	resp = jsonPost(t, ts.URL+"/v1/sessions", sessReq)
	assertStatus(t, resp, http.StatusCreated)

	var sess session.Session
	decodeJSON(t, resp, &sess)
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.Status != session.StatusStarting {
		t.Fatalf("expected status 'starting', got %q", sess.Status)
	}
	if sess.Agent != ag.ID {
		t.Fatalf("expected agent_id %q, got %q", ag.ID, sess.Agent)
	}

	sessionID := sess.ID

	// 2. GET /v1/sessions -> list -> contains it
	resp = jsonGet(t, ts.URL+"/v1/sessions")
	assertStatus(t, resp, http.StatusOK)

	var sessions []*session.Session
	decodeJSON(t, resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != sessionID {
		t.Fatalf("expected session ID %q, got %q", sessionID, sessions[0].ID)
	}

	// 3. GET /v1/sessions/:id -> get -> verify fields
	resp = jsonGet(t, ts.URL+"/v1/sessions/"+sessionID)
	assertStatus(t, resp, http.StatusOK)

	var fetchedSess session.Session
	decodeJSON(t, resp, &fetchedSess)
	if fetchedSess.ID != sessionID {
		t.Fatalf("expected session ID %q, got %q", sessionID, fetchedSess.ID)
	}
	if fetchedSess.EnvironmentID != env.ID {
		t.Fatalf("expected environment_id %q, got %q", env.ID, fetchedSess.EnvironmentID)
	}

	// 4. POST /v1/sessions/:id/events -> post user message -> 202
	eventReq := session.UserMessageEvent{
		Type: "user_message",
		Content: []session.ContentBlock{
			{Type: "text", Text: "Hello from e2e test"},
		},
	}
	resp = jsonPost(t, ts.URL+"/v1/sessions/"+sessionID+"/events", eventReq)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// 5. GET /v1/sessions/:id/events -> verify event stored
	resp = jsonGet(t, ts.URL+"/v1/sessions/"+sessionID+"/events")
	assertStatus(t, resp, http.StatusOK)

	var events []*store.StoredEvent
	decodeJSON(t, resp, &events)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "user_message" {
		t.Fatalf("expected event type 'user_message', got %q", events[0].Type)
	}
	if events[0].SessionID != sessionID {
		t.Fatalf("expected event session_id %q, got %q", sessionID, events[0].SessionID)
	}
}

// ---------------------------------------------------------------------------
// Test: SSE streaming
// ---------------------------------------------------------------------------

func TestE2E_SSEStreaming(t *testing.T) {
	srv, ts := setupE2EServer(t)

	// Create agent + environment + session
	agentReq := agent.CreateRequest{
		Name:  "sse-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	resp := jsonPost(t, ts.URL+"/v1/agents", agentReq)
	assertStatus(t, resp, http.StatusCreated)
	var ag agent.Agent
	decodeJSON(t, resp, &ag)

	envReq := environment.CreateRequest{
		Name: "sse-env",
		Config: environment.EnvironmentConfig{
			Type:       "local",
			Networking: environment.NetworkConfig{Type: "full"},
		},
	}
	resp = jsonPost(t, ts.URL+"/v1/environments", envReq)
	assertStatus(t, resp, http.StatusCreated)
	var env environment.Environment
	decodeJSON(t, resp, &env)

	sessReq := session.CreateRequest{
		AgentID:       ag.ID,
		EnvironmentID: env.ID,
	}
	resp = jsonPost(t, ts.URL+"/v1/sessions", sessReq)
	assertStatus(t, resp, http.StatusCreated)
	var sess session.Session
	decodeJSON(t, resp, &sess)

	sessionID := sess.ID

	// Channel to collect received SSE events
	received := make(chan session.Event, 10)
	sseReady := make(chan struct{})
	sseDone := make(chan struct{})

	// In a goroutine, connect to SSE stream
	go func() {
		defer close(sseDone)
		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/sessions/"+sessionID+"/stream", nil)
		if err != nil {
			t.Errorf("create SSE request: %v", err)
			return
		}

		sseResp, err := client.Do(req)
		if err != nil {
			// Timeout is expected after we're done
			return
		}
		defer sseResp.Body.Close()

		if sseResp.StatusCode != http.StatusOK {
			t.Errorf("expected SSE status 200, got %d", sseResp.StatusCode)
			return
		}

		close(sseReady) // Signal that SSE connection is established

		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var evt session.Event
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					t.Errorf("unmarshal SSE event: %v", err)
					continue
				}
				received <- evt
			}
		}
	}()

	// Wait for SSE connection to be ready (or timeout)
	select {
	case <-sseReady:
		// SSE connection established
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE connection")
	}

	// Give the SSE handler time to register the subscription
	time.Sleep(50 * time.Millisecond)

	// Emit events directly via eventBus
	testContent, _ := json.Marshal(map[string]string{"text": "hello from SSE test"})
	srv.eventBus.Emit(sessionID, session.Event{
		Type:    "test_event",
		Content: testContent,
	})
	srv.eventBus.Emit(sessionID, session.Event{
		Type:    "test_event_2",
		Content: testContent,
	})

	// Verify the SSE client receives the events (with timeout)
	var receivedEvents []session.Event
	timeout := time.After(3 * time.Second)

	for i := 0; i < 2; i++ {
		select {
		case evt := <-received:
			receivedEvents = append(receivedEvents, evt)
		case <-timeout:
			t.Fatalf("timeout waiting for SSE events, received %d of 2", len(receivedEvents))
		}
	}

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 SSE events, got %d", len(receivedEvents))
	}
	if receivedEvents[0].Type != "test_event" {
		t.Errorf("expected first event type 'test_event', got %q", receivedEvents[0].Type)
	}
	if receivedEvents[1].Type != "test_event_2" {
		t.Errorf("expected second event type 'test_event_2', got %q", receivedEvents[1].Type)
	}

	fmt.Println("SSE streaming test passed: received", len(receivedEvents), "events")
}

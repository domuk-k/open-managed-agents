//go:build integration

package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

// TestE2BIntegration_CreateExecDestroy tests the full lifecycle using a mock server.
func TestE2BIntegration_CreateExecDestroy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			json.NewEncoder(w).Encode(e2bCreateResponse{
				SandboxID: "sbx-integ-1",
				ClientID:  "cli-integ-1",
			})

		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-integ-1/processes":
			var req e2bExecRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(e2bExecResponse{
				Stdout:   "integration test output",
				ExitCode: 0,
			})

		case r.Method == http.MethodDelete && r.URL.Path == "/sandboxes/sbx-integ-1":
			w.WriteHeader(http.StatusNoContent)

		default:
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewE2BProvider("integ-key", WithE2BBaseURL(server.URL))
	ctx := context.Background()

	// Create.
	sb, err := provider.Create(ctx, environment.EnvironmentConfig{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sb.ID() != "sbx-integ-1" {
		t.Fatalf("expected ID 'sbx-integ-1', got %q", sb.ID())
	}

	// Exec.
	result, err := sb.Exec(ctx, "echo test", ExecOpts{})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.Stdout != "integration test output" {
		t.Errorf("unexpected stdout: %q", result.Stdout)
	}

	// Destroy.
	if err := sb.Destroy(ctx); err != nil {
		t.Fatalf("destroy: %v", err)
	}
}

// TestE2BIntegration_WriteReadFile tests write + read file round trip.
func TestE2BIntegration_WriteReadFile(t *testing.T) {
	fileStore := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-files/files":
			var req e2bWriteFileRequest
			json.NewDecoder(r.Body).Decode(&req)
			fileStore[req.Path] = req.Content
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sbx-files/files":
			path := r.URL.Query().Get("path")
			content, ok := fileStore[path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(e2bErrorResponse{Message: "file not found"})
				return
			}
			w.Write([]byte(content))

		default:
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewE2BProvider("integ-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-files", provider: provider}
	ctx := context.Background()

	// Write.
	if err := sb.WriteFile(ctx, "/workspace/hello.txt", []byte("hello e2b")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Read.
	data, err := sb.ReadFile(ctx, "/workspace/hello.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello e2b" {
		t.Errorf("expected 'hello e2b', got %q", string(data))
	}
}

package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

func TestE2BProvider_Create(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			// Verify headers.
			if r.Header.Get("X-API-Key") != "test-key" {
				t.Errorf("expected X-API-Key header 'test-key', got %q", r.Header.Get("X-API-Key"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
			}

			// Verify request body.
			var req e2bCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Template != "base" {
				t.Errorf("expected template 'base', got %q", req.Template)
			}
			if req.Timeout != 300 {
				t.Errorf("expected timeout 300, got %d", req.Timeout)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bCreateResponse{
				SandboxID: "sbx-123",
				ClientID:  "cli-456",
			})

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb, err := provider.Create(context.Background(), defaultEnvConfig())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sb.ID() != "sbx-123" {
		t.Errorf("expected ID 'sbx-123', got %q", sb.ID())
	}
}

func TestE2BProvider_Destroy(t *testing.T) {
	var deleteCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/sandboxes/sbx-123" {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))

	// Seed the sandbox in the provider's map.
	provider.mu.Lock()
	provider.sandboxes["sbx-123"] = &E2BSandbox{id: "sbx-123", provider: provider}
	provider.mu.Unlock()

	err := provider.Destroy(context.Background(), "sbx-123")
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !deleteCalled {
		t.Error("expected DELETE to be called")
	}
}

func TestE2BSandbox_Exec(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-123/processes" {
			var req e2bExecRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Cmd != "echo hello" {
				t.Errorf("expected cmd 'echo hello', got %q", req.Cmd)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bExecResponse{
				Stdout:   "hello\n",
				Stderr:   "",
				ExitCode: 0,
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	result, err := sb.Exec(context.Background(), "echo hello", ExecOpts{})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestE2BSandbox_WriteFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-123/files" {
			var req e2bWriteFileRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Path != "/tmp/test.txt" {
				t.Errorf("expected path '/tmp/test.txt', got %q", req.Path)
			}
			if req.Content != "hello world" {
				t.Errorf("expected content 'hello world', got %q", req.Content)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	err := sb.WriteFile(context.Background(), "/tmp/test.txt", []byte("hello world"))
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestE2BSandbox_ReadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sbx-123/files" {
			if r.URL.Query().Get("path") != "/tmp/test.txt" {
				t.Errorf("expected query path '/tmp/test.txt', got %q", r.URL.Query().Get("path"))
			}
			w.Write([]byte("file contents here"))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	data, err := sb.ReadFile(context.Background(), "/tmp/test.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "file contents here" {
		t.Errorf("expected 'file contents here', got %q", string(data))
	}
}

func TestE2BSandbox_Glob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-123/processes" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bExecResponse{
				Stdout:   "/tmp/a.txt\n/tmp/b.txt\n",
				ExitCode: 0,
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	files, err := sb.Glob(context.Background(), "/tmp/*.txt")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "/tmp/a.txt" || files[1] != "/tmp/b.txt" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestE2BSandbox_Grep(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-123/processes" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bExecResponse{
				Stdout:   "/tmp/test.go:10:func main() {\n/tmp/test.go:15:func helper() {\n",
				ExitCode: 0,
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	matches, err := sb.Grep(context.Background(), "func", "/tmp")
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Path != "/tmp/test.go" || matches[0].Line != 10 || matches[0].Content != "func main() {" {
		t.Errorf("unexpected match[0]: %+v", matches[0])
	}
}

func TestE2BProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(e2bErrorResponse{
			Message: "invalid api key",
			Code:    401,
		})
	}))
	defer server.Close()

	provider := NewE2BProvider("bad-key", WithE2BBaseURL(server.URL))
	_, err := provider.Create(context.Background(), defaultEnvConfig())
	if err == nil {
		t.Fatal("expected error for bad API key")
	}
	if !contains(err.Error(), "invalid api key") {
		t.Errorf("expected error to contain 'invalid api key', got: %v", err)
	}
}

func TestE2BSandbox_ExecWithCwdAndEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-123/processes" {
			var req e2bExecRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Cwd != "/workspace" {
				t.Errorf("expected cwd '/workspace', got %q", req.Cwd)
			}
			if req.Env["FOO"] != "bar" {
				t.Errorf("expected env FOO=bar, got %q", req.Env["FOO"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bExecResponse{
				Stdout:   "ok",
				ExitCode: 0,
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))
	sb := &E2BSandbox{id: "sbx-123", provider: provider}

	result, err := sb.Exec(context.Background(), "pwd", ExecOpts{
		Cwd: "/workspace",
		Env: map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.Stdout != "ok" {
		t.Errorf("unexpected stdout: %q", result.Stdout)
	}
}

func TestE2BProvider_CreateDestroyCleansMap(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			calls++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(e2bCreateResponse{
				SandboxID: "sbx-abc",
				ClientID:  "cli-def",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/sandboxes/sbx-abc":
			calls++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewE2BProvider("test-key", WithE2BBaseURL(server.URL))

	sb, err := provider.Create(context.Background(), defaultEnvConfig())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	provider.mu.Lock()
	_, exists := provider.sandboxes["sbx-abc"]
	provider.mu.Unlock()
	if !exists {
		t.Error("sandbox should be in map after Create")
	}

	if err := sb.Destroy(context.Background()); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	provider.mu.Lock()
	_, exists = provider.sandboxes["sbx-abc"]
	provider.mu.Unlock()
	if exists {
		t.Error("sandbox should be removed from map after Destroy")
	}

	if calls != 2 {
		t.Errorf("expected 2 API calls, got %d", calls)
	}
}

// helpers

func defaultEnvConfig() environment.EnvironmentConfig {
	return environment.EnvironmentConfig{}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

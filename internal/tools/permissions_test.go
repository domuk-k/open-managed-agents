package tools

import (
	"encoding/json"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
)

func TestPermissionChecker_NilPolicy(t *testing.T) {
	pc := NewPermissionChecker(nil)
	err := pc.Check("bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("nil policy should allow everything, got: %v", err)
	}
}

func TestPermissionChecker_AlwaysAllow(t *testing.T) {
	pc := NewPermissionChecker(&agent.PermissionPolicy{Type: "always_allow"})
	err := pc.Check("bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("always_allow should allow everything, got: %v", err)
	}
}

func TestPermissionChecker_ScopedBashAllowed(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{
				Tool:  "bash",
				Allow: true,
				Constraints: &agent.Constraints{
					Commands: []string{"ls", "cat ", "git "},
				},
			},
		},
	}
	pc := NewPermissionChecker(policy)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"exact prefix match", `{"command":"ls -la"}`, false},
		{"cat with space prefix", `{"command":"cat /etc/passwd"}`, false},
		{"git with space prefix", `{"command":"git status"}`, false},
		{"denied command", `{"command":"rm -rf /"}`, true},
		{"partial prefix no match", `{"command":"echo hello"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pc.Check("bash", json.RawMessage(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPermissionChecker_ScopedDenyUnlisted(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{Tool: "bash", Allow: true},
		},
	}
	pc := NewPermissionChecker(policy)

	// bash is listed and allowed.
	if err := pc.Check("bash", json.RawMessage(`{"command":"ls"}`)); err != nil {
		t.Fatalf("bash should be allowed, got: %v", err)
	}

	// file_read is not listed, should be denied.
	err := pc.Check("file_read", json.RawMessage(`{"path":"/tmp/x"}`))
	if err == nil {
		t.Fatal("unlisted tool should be denied")
	}
}

func TestPermissionChecker_ScopedExplicitDeny(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{Tool: "bash", Allow: false},
		},
	}
	pc := NewPermissionChecker(policy)

	err := pc.Check("bash", json.RawMessage(`{"command":"ls"}`))
	if err == nil {
		t.Fatal("explicitly denied tool should return error")
	}
}

func TestPermissionChecker_FilePathConstraints(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{
				Tool:  "file_read",
				Allow: true,
				Constraints: &agent.Constraints{
					Paths: []string{"/workspace/", "/tmp/"},
				},
			},
			{
				Tool:  "file_write",
				Allow: true,
				Constraints: &agent.Constraints{
					Paths: []string{"/workspace/"},
				},
			},
			{
				Tool:  "file_edit",
				Allow: true,
				Constraints: &agent.Constraints{
					Paths: []string{"/workspace/src/"},
				},
			},
		},
	}
	pc := NewPermissionChecker(policy)

	tests := []struct {
		name    string
		tool    string
		input   string
		wantErr bool
	}{
		{"read under workspace", "file_read", `{"path":"/workspace/foo.txt"}`, false},
		{"read under tmp", "file_read", `{"path":"/tmp/bar.txt"}`, false},
		{"read outside allowed", "file_read", `{"path":"/etc/passwd"}`, true},
		{"write under workspace", "file_write", `{"path":"/workspace/out.txt","content":"x"}`, false},
		{"write outside allowed", "file_write", `{"path":"/tmp/out.txt","content":"x"}`, true},
		{"edit under src", "file_edit", `{"path":"/workspace/src/main.go","old_string":"a","new_string":"b"}`, false},
		{"edit outside src", "file_edit", `{"path":"/workspace/README.md","old_string":"a","new_string":"b"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pc.Check(tt.tool, json.RawMessage(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Check(%s) error = %v, wantErr %v", tt.tool, err, tt.wantErr)
			}
		})
	}
}

func TestPermissionChecker_WebDomainConstraints(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{
				Tool:  "web_fetch",
				Allow: true,
				Constraints: &agent.Constraints{
					Domains: []string{"example.com", "api.github.com"},
				},
			},
			{
				Tool:  "web_search",
				Allow: true,
				Constraints: &agent.Constraints{
					Domains: []string{"google.com"},
				},
			},
		},
	}
	pc := NewPermissionChecker(policy)

	tests := []struct {
		name    string
		tool    string
		input   string
		wantErr bool
	}{
		{"fetch allowed domain", "web_fetch", `{"url":"https://example.com/page"}`, false},
		{"fetch subdomain match", "web_fetch", `{"url":"https://sub.example.com/page"}`, false},
		{"fetch exact api.github.com", "web_fetch", `{"url":"https://api.github.com/repos"}`, false},
		{"fetch denied domain", "web_fetch", `{"url":"https://evil.com/steal"}`, true},
		{"search always allowed", "web_search", `{"query":"golang tutorials"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pc.Check(tt.tool, json.RawMessage(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Check(%s) error = %v, wantErr %v", tt.tool, err, tt.wantErr)
			}
		})
	}
}

func TestPermissionChecker_AllowWithNoConstraints(t *testing.T) {
	policy := &agent.PermissionPolicy{
		Type: "scoped",
		Scopes: []agent.ToolScope{
			{Tool: "glob", Allow: true},
		},
	}
	pc := NewPermissionChecker(policy)

	err := pc.Check("glob", json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatalf("tool with allow=true and no constraints should be permitted, got: %v", err)
	}
}

//go:build integration

package sandbox_test

import (
	"context"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

func TestDockerCreateExecDestroy(t *testing.T) {
	provider, err := sandbox.NewDockerProvider()
	if err != nil {
		t.Fatalf("NewDockerProvider: %v", err)
	}

	ctx := context.Background()
	sb, err := provider.Create(ctx, environment.EnvironmentConfig{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer sb.Destroy(ctx)

	result, err := sb.Exec(ctx, "echo hello", sandbox.ExecOpts{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if got := result.Stdout; got != "hello\n" {
		t.Errorf("expected stdout %q, got %q", "hello\n", got)
	}
}

func TestDockerWriteReadFile(t *testing.T) {
	provider, err := sandbox.NewDockerProvider()
	if err != nil {
		t.Fatalf("NewDockerProvider: %v", err)
	}

	ctx := context.Background()
	sb, err := provider.Create(ctx, environment.EnvironmentConfig{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer sb.Destroy(ctx)

	content := []byte("hello, sandbox!")
	path := "/tmp/test-file.txt"

	if err := sb.WriteFile(ctx, path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := sb.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("ReadFile roundtrip: expected %q, got %q", string(content), string(got))
	}
}

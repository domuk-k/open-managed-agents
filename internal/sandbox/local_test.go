package sandbox

import (
	"context"
	"os"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

func newTestSandbox(t *testing.T) (*LocalProvider, Sandbox) {
	t.Helper()
	p := NewLocalProvider()
	sb, err := p.Create(context.Background(), environment.EnvironmentConfig{})
	if err != nil {
		t.Fatalf("Create sandbox: %v", err)
	}
	return p, sb
}

func TestExec(t *testing.T) {
	_, sb := newTestSandbox(t)
	defer sb.Destroy(context.Background())

	res, err := sb.Exec(context.Background(), "echo hello", ExecOpts{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("expected stdout %q, got %q", "hello\n", res.Stdout)
	}
}

func TestExecNonZeroExit(t *testing.T) {
	_, sb := newTestSandbox(t)
	defer sb.Destroy(context.Background())

	res, err := sb.Exec(context.Background(), "exit 42", ExecOpts{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
}

func TestWriteFileReadFile(t *testing.T) {
	_, sb := newTestSandbox(t)
	defer sb.Destroy(context.Background())

	ctx := context.Background()
	content := []byte("hello world")

	if err := sb.WriteFile(ctx, "subdir/test.txt", content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := sb.ReadFile(ctx, "subdir/test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestGlob(t *testing.T) {
	_, sb := newTestSandbox(t)
	defer sb.Destroy(context.Background())

	ctx := context.Background()
	sb.WriteFile(ctx, "a.txt", []byte("a"))
	sb.WriteFile(ctx, "b.txt", []byte("b"))
	sb.WriteFile(ctx, "c.go", []byte("c"))

	matches, err := sb.Glob(ctx, "*.txt")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestGrep(t *testing.T) {
	_, sb := newTestSandbox(t)
	defer sb.Destroy(context.Background())

	ctx := context.Background()
	sb.WriteFile(ctx, "file.txt", []byte("line one\nline two\nfoo bar\n"))

	matches, err := sb.Grep(ctx, "foo", "file.txt")
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Line != 3 {
		t.Errorf("expected line 3, got %d", matches[0].Line)
	}
	if matches[0].Content != "foo bar" {
		t.Errorf("expected content %q, got %q", "foo bar", matches[0].Content)
	}
}

func TestDestroy(t *testing.T) {
	p, sb := newTestSandbox(t)
	ctx := context.Background()

	sb.WriteFile(ctx, "test.txt", []byte("data"))

	// Get the sandbox root via ID to check cleanup
	ls := sb.(*LocalSandbox)
	root := ls.root

	if err := p.Destroy(ctx, sb.ID()); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("expected sandbox dir to be removed, but it still exists")
	}
}

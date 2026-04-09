package sandbox

import (
	"context"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

type Provider interface {
	Create(ctx context.Context, config environment.EnvironmentConfig) (Sandbox, error)
	Destroy(ctx context.Context, id string) error
}

type Sandbox interface {
	ID() string
	Exec(ctx context.Context, cmd string, opts ExecOpts) (*ExecResult, error)
	WriteFile(ctx context.Context, path string, content []byte) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	Glob(ctx context.Context, pattern string) ([]string, error)
	Grep(ctx context.Context, pattern string, path string) ([]GrepMatch, error)
	Destroy(ctx context.Context) error
}

type ExecOpts struct {
	Timeout time.Duration
	Cwd     string
	Env     map[string]string
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type GrepMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

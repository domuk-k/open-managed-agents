package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

// LocalProvider creates sandboxes backed by local temp directories.
type LocalProvider struct {
	mu        sync.Mutex
	sandboxes map[string]*LocalSandbox
}

// NewLocalProvider returns a new LocalProvider.
func NewLocalProvider() *LocalProvider {
	return &LocalProvider{
		sandboxes: make(map[string]*LocalSandbox),
	}
}

func (p *LocalProvider) Create(ctx context.Context, config environment.EnvironmentConfig) (Sandbox, error) {
	dir, err := os.MkdirTemp("", "oma-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("create sandbox temp dir: %w", err)
	}

	id := filepath.Base(dir)
	sb := &LocalSandbox{
		id:   id,
		root: dir,
	}

	p.mu.Lock()
	p.sandboxes[id] = sb
	p.mu.Unlock()

	return sb, nil
}

func (p *LocalProvider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	sb, ok := p.sandboxes[id]
	if ok {
		delete(p.sandboxes, id)
	}
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("sandbox %q not found", id)
	}
	return sb.Destroy(ctx)
}

// LocalSandbox is a sandbox backed by a local temp directory.
type LocalSandbox struct {
	id   string
	root string
}

func (s *LocalSandbox) ID() string {
	return s.id
}

func (s *LocalSandbox) Exec(ctx context.Context, cmd string, opts ExecOpts) (*ExecResult, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, "sh", "-c", cmd)

	cwd := s.root
	if opts.Cwd != "" {
		cwd = s.resolvePath(opts.Cwd)
	}
	c.Dir = cwd

	for k, v := range opts.Env {
		c.Env = append(c.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, err
		}
	}

	return result, nil
}

func (s *LocalSandbox) WriteFile(ctx context.Context, path string, content []byte) error {
	full := s.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	return os.WriteFile(full, content, 0o644)
}

func (s *LocalSandbox) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(s.resolvePath(path))
}

func (s *LocalSandbox) Glob(ctx context.Context, pattern string) ([]string, error) {
	full := s.resolvePath(pattern)
	matches, err := filepath.Glob(full)
	if err != nil {
		return nil, err
	}
	// Return paths relative to sandbox root.
	rel := make([]string, 0, len(matches))
	for _, m := range matches {
		r, err := filepath.Rel(s.root, m)
		if err != nil {
			rel = append(rel, m)
		} else {
			rel = append(rel, r)
		}
	}
	return rel, nil
}

func (s *LocalSandbox) Grep(ctx context.Context, pattern string, path string) ([]GrepMatch, error) {
	target := s.resolvePath(path)

	cmd := exec.CommandContext(ctx, "grep", "-rn", pattern, target)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	_ = cmd.Run() // grep returns exit 1 on no match; ignore error

	var matches []GrepMatch
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: filepath:linenum:content
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		relPath, err := filepath.Rel(s.root, parts[0])
		if err != nil {
			relPath = parts[0]
		}
		matches = append(matches, GrepMatch{
			Path:    relPath,
			Line:    lineNum,
			Content: parts[2],
		})
	}
	return matches, nil
}

func (s *LocalSandbox) Destroy(ctx context.Context) error {
	return os.RemoveAll(s.root)
}

// resolvePath returns an absolute path. If path is already absolute, return as-is;
// otherwise join with sandbox root.
func (s *LocalSandbox) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.root, path)
}

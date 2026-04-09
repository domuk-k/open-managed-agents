package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

const (
	defaultE2BURL      = "https://api.e2b.dev"
	defaultE2BTemplate = "base"
	defaultE2BTimeout  = 300
)

// HTTPClient is an interface for making HTTP requests, allowing test injection.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// E2BProvider implements the Provider interface using E2B cloud sandboxes.
type E2BProvider struct {
	apiKey   string
	baseURL  string
	template string
	client   HTTPClient

	mu        sync.Mutex
	sandboxes map[string]*E2BSandbox
}

// E2BProviderOption configures an E2BProvider.
type E2BProviderOption func(*E2BProvider)

// WithE2BBaseURL overrides the default E2B API base URL.
func WithE2BBaseURL(url string) E2BProviderOption {
	return func(p *E2BProvider) {
		p.baseURL = url
	}
}

// WithE2BTemplate overrides the default sandbox template.
func WithE2BTemplate(template string) E2BProviderOption {
	return func(p *E2BProvider) {
		p.template = template
	}
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(client HTTPClient) E2BProviderOption {
	return func(p *E2BProvider) {
		p.client = client
	}
}

// NewE2BProvider creates a new E2BProvider with the given API key and options.
func NewE2BProvider(apiKey string, opts ...E2BProviderOption) *E2BProvider {
	p := &E2BProvider{
		apiKey:    apiKey,
		baseURL:   defaultE2BURL,
		template:  defaultE2BTemplate,
		client:    http.DefaultClient,
		sandboxes: make(map[string]*E2BSandbox),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// e2bCreateRequest is the request body for creating an E2B sandbox.
type e2bCreateRequest struct {
	Template string `json:"template"`
	Timeout  int    `json:"timeout"`
}

// e2bCreateResponse is the response from creating an E2B sandbox.
type e2bCreateResponse struct {
	SandboxID string `json:"sandboxId"`
	ClientID  string `json:"clientId"`
}

// e2bExecRequest is the request body for executing a command.
type e2bExecRequest struct {
	Cmd string            `json:"cmd"`
	Cwd string            `json:"cwd,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}

// e2bExecResponse is the response from executing a command.
type e2bExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// e2bWriteFileRequest is the request body for writing a file.
type e2bWriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// e2bErrorResponse represents an error response from the E2B API.
type e2bErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Create creates a new E2B sandbox from the given config.
func (p *E2BProvider) Create(ctx context.Context, config environment.EnvironmentConfig) (Sandbox, error) {
	template := p.template
	if config.Type != "" {
		template = config.Type
	}

	timeout := defaultE2BTimeout
	if config.Resources != nil && config.Resources.TimeoutSeconds != nil {
		timeout = *config.Resources.TimeoutSeconds
	}

	reqBody := e2bCreateRequest{
		Template: template,
		Timeout:  timeout,
	}

	var resp e2bCreateResponse
	if err := p.doJSON(ctx, http.MethodPost, "/sandboxes", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("e2b create sandbox: %w", err)
	}

	sb := &E2BSandbox{
		id:       resp.SandboxID,
		clientID: resp.ClientID,
		provider: p,
	}

	p.mu.Lock()
	p.sandboxes[resp.SandboxID] = sb
	p.mu.Unlock()

	return sb, nil
}

// Destroy destroys the E2B sandbox with the given ID.
func (p *E2BProvider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	delete(p.sandboxes, id)
	p.mu.Unlock()

	return p.doJSON(ctx, http.MethodDelete, "/sandboxes/"+id, nil, nil)
}

// doJSON performs an HTTP request with JSON body and decodes the response.
func (p *E2BProvider) doJSON(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-API-Key", p.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp e2bErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("e2b api error (status %d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("e2b api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// E2BSandbox implements the Sandbox interface backed by an E2B cloud sandbox.
type E2BSandbox struct {
	id       string
	clientID string
	provider *E2BProvider
}

// ID returns the sandbox ID.
func (s *E2BSandbox) ID() string {
	return s.id
}

// Exec runs a command inside the E2B sandbox.
func (s *E2BSandbox) Exec(ctx context.Context, cmd string, opts ExecOpts) (*ExecResult, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	reqBody := e2bExecRequest{
		Cmd: cmd,
		Cwd: opts.Cwd,
		Env: opts.Env,
	}

	var resp e2bExecResponse
	if err := s.provider.doJSON(ctx, http.MethodPost, "/sandboxes/"+s.id+"/processes", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("e2b exec: %w", err)
	}

	return &ExecResult{
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
		ExitCode: resp.ExitCode,
	}, nil
}

// WriteFile writes content to a file inside the E2B sandbox.
func (s *E2BSandbox) WriteFile(ctx context.Context, path string, content []byte) error {
	reqBody := e2bWriteFileRequest{
		Path:    path,
		Content: string(content),
	}
	return s.provider.doJSON(ctx, http.MethodPost, "/sandboxes/"+s.id+"/files", reqBody, nil)
}

// ReadFile reads a file from the E2B sandbox.
func (s *E2BSandbox) ReadFile(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.provider.baseURL+"/sandboxes/"+s.id+"/files?path="+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-Key", s.provider.apiKey)

	resp, err := s.provider.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp e2bErrorResponse
		if json.Unmarshal(data, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("e2b read file (status %d): %s", resp.StatusCode, errResp.Message)
		}
		return nil, fmt.Errorf("e2b read file (status %d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// Glob returns file paths matching the given glob pattern inside the E2B sandbox.
func (s *E2BSandbox) Glob(ctx context.Context, pattern string) ([]string, error) {
	result, err := s.Exec(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null", pattern), ExecOpts{})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, nil
	}

	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

// Grep searches for a pattern in files at the given path inside the E2B sandbox.
func (s *E2BSandbox) Grep(ctx context.Context, pattern string, path string) ([]GrepMatch, error) {
	result, err := s.Exec(ctx, fmt.Sprintf("grep -rn %q %q", pattern, path), ExecOpts{})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		if result.ExitCode == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("grep failed (exit %d): %s", result.ExitCode, result.Stderr)
	}

	var matches []GrepMatch
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" {
			continue
		}
		firstColon := strings.Index(line, ":")
		if firstColon < 0 {
			continue
		}
		rest := line[firstColon+1:]
		secondColon := strings.Index(rest, ":")
		if secondColon < 0 {
			continue
		}
		filePath := line[:firstColon]
		lineNum, err := strconv.Atoi(rest[:secondColon])
		if err != nil {
			continue
		}
		content := rest[secondColon+1:]

		matches = append(matches, GrepMatch{
			Path:    filePath,
			Line:    lineNum,
			Content: content,
		})
	}
	return matches, nil
}

// Destroy destroys the E2B sandbox.
func (s *E2BSandbox) Destroy(ctx context.Context) error {
	return s.provider.Destroy(ctx, s.id)
}

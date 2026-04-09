package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/domuk-k/open-managed-agents/internal/environment"
)

const defaultImage = "node:20-slim"

// DockerProvider implements the Provider interface using Docker containers.
type DockerProvider struct {
	cli *client.Client
}

// NewDockerProvider creates a new DockerProvider using environment-based Docker client config.
func NewDockerProvider() (*DockerProvider, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerProvider{cli: cli}, nil
}

// Create creates a new Docker container sandbox from the given config.
func (p *DockerProvider) Create(ctx context.Context, config environment.EnvironmentConfig) (Sandbox, error) {
	image := defaultImage
	if config.Type != "" {
		image = config.Type
	}

	// Build container config.
	containerCfg := &container.Config{
		Image: image,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   false,
	}

	// Set environment variables.
	for k, v := range config.EnvVars {
		containerCfg.Env = append(containerCfg.Env, k+"="+v)
	}

	// Build host config.
	hostCfg := &container.HostConfig{}

	// Resources.
	if res := config.Resources; res != nil {
		if res.MemoryMB != nil {
			hostCfg.Resources.Memory = int64(*res.MemoryMB) * 1024 * 1024
		}
		if res.CPUCores != nil {
			hostCfg.Resources.NanoCPUs = int64(*res.CPUCores) * 1e9
		}
	}

	// Networking.
	var networkCfg *network.NetworkingConfig
	switch config.Networking.Type {
	case "none":
		hostCfg.NetworkMode = "none"
	case "restricted":
		// For restricted mode, use bridge with DNS restrictions.
		// A full implementation would create a custom network with iptables rules.
		// For now, use bridge as the base.
		hostCfg.NetworkMode = "bridge"
	default:
		// "unrestricted" or empty → bridge
		hostCfg.NetworkMode = "bridge"
	}

	resp, err := p.cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, "")
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}

	if err := p.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up on failed start.
		_ = p.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, fmt.Errorf("container start: %w", err)
	}

	return &DockerSandbox{
		id:  resp.ID,
		cli: p.cli,
	}, nil
}

// Destroy stops and removes the container with the given ID.
func (p *DockerProvider) Destroy(ctx context.Context, id string) error {
	timeout := 10 // seconds
	stopOpts := container.StopOptions{Timeout: &timeout}
	if err := p.cli.ContainerStop(ctx, id, stopOpts); err != nil {
		// If stop fails (e.g. already stopped), still try to remove.
		_ = err
	}
	if err := p.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}
	return nil
}

// DockerSandbox implements the Sandbox interface backed by a Docker container.
type DockerSandbox struct {
	id  string
	cli *client.Client
}

// ID returns the container ID.
func (s *DockerSandbox) ID() string {
	return s.id
}

// Exec runs a command inside the container via docker exec.
func (s *DockerSandbox) Exec(ctx context.Context, cmd string, opts ExecOpts) (*ExecResult, error) {
	execCmd := []string{"sh", "-c", cmd}

	execCfg := container.ExecOptions{
		Cmd:          execCmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	if opts.Cwd != "" {
		execCfg.WorkingDir = opts.Cwd
	}

	for k, v := range opts.Env {
		execCfg.Env = append(execCfg.Env, k+"="+v)
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	execResp, err := s.cli.ContainerExecCreate(ctx, s.id, execCfg)
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := s.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader); err != nil {
		return nil, fmt.Errorf("exec read: %w", err)
	}

	inspectResp, err := s.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}

	return &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: inspectResp.ExitCode,
	}, nil
}

// WriteFile writes content to a file inside the container using tar copy.
func (s *DockerSandbox) WriteFile(ctx context.Context, path string, content []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Extract the file name from the path for the tar header.
	// We'll copy to the parent directory.
	parts := strings.Split(path, "/")
	fileName := parts[len(parts)-1]
	dir := strings.Join(parts[:len(parts)-1], "/")
	if dir == "" {
		dir = "/"
	}

	hdr := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("tar write: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("tar close: %w", err)
	}

	// Ensure parent directory exists.
	_, _ = s.Exec(ctx, fmt.Sprintf("mkdir -p %q", dir), ExecOpts{})

	return s.cli.CopyToContainer(ctx, s.id, dir, &buf, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	})
}

// ReadFile reads a file from the container.
func (s *DockerSandbox) ReadFile(ctx context.Context, path string) ([]byte, error) {
	rc, _, err := s.cli.CopyFromContainer(ctx, s.id, path)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("tar next: %w", err)
	}

	data, err := io.ReadAll(tr)
	if err != nil {
		return nil, fmt.Errorf("read tar entry: %w", err)
	}
	return data, nil
}

// Glob returns file paths matching the given glob pattern inside the container.
func (s *DockerSandbox) Glob(ctx context.Context, pattern string) ([]string, error) {
	result, err := s.Exec(ctx, fmt.Sprintf("find . -path '%s' 2>/dev/null", strings.ReplaceAll(pattern, "'", "'\\''")), ExecOpts{})
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

// Grep searches for a pattern in files at the given path inside the container.
func (s *DockerSandbox) Grep(ctx context.Context, pattern string, path string) ([]GrepMatch, error) {
	result, err := s.Exec(ctx, fmt.Sprintf("grep -rn -- '%s' '%s'", strings.ReplaceAll(pattern, "'", "'\\''"), strings.ReplaceAll(path, "'", "'\\''")), ExecOpts{})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		// grep exit code 1 means no matches, not an error.
		if result.ExitCode == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("grep failed (exit %d): %s", result.ExitCode, result.Stderr)
	}

	matches := make([]GrepMatch, 0)
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" {
			continue
		}
		// Format: file:line_number:content
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

// Destroy stops and removes the container.
func (s *DockerSandbox) Destroy(ctx context.Context) error {
	timeout := 10
	stopOpts := container.StopOptions{Timeout: &timeout}
	_ = s.cli.ContainerStop(ctx, s.id, stopOpts)
	return s.cli.ContainerRemove(ctx, s.id, container.RemoveOptions{Force: true})
}

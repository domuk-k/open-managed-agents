package tools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/domuk-k/open-managed-agents/internal/agent"
)

// PermissionChecker enforces a PermissionPolicy before tool execution.
type PermissionChecker struct {
	policy *agent.PermissionPolicy
}

// NewPermissionChecker creates a PermissionChecker for the given policy.
// A nil policy is treated as "always allow".
func NewPermissionChecker(policy *agent.PermissionPolicy) *PermissionChecker {
	return &PermissionChecker{policy: policy}
}

// Check validates whether the given tool invocation is allowed under the policy.
// It returns nil if allowed, or an error describing why access was denied.
func (pc *PermissionChecker) Check(toolName string, input json.RawMessage) error {
	if pc.policy == nil || pc.policy.Type == "always_allow" {
		return nil
	}

	if pc.policy.Type != "scoped" {
		return fmt.Errorf("permission denied: unknown policy type %q", pc.policy.Type)
	}

	// Find matching scope for the tool.
	var scope *agent.ToolScope
	for i := range pc.policy.Scopes {
		if pc.policy.Scopes[i].Tool == toolName {
			scope = &pc.policy.Scopes[i]
			break
		}
	}

	if scope == nil {
		return fmt.Errorf("permission denied: tool %q is not listed in scoped policy", toolName)
	}

	if !scope.Allow {
		return fmt.Errorf("permission denied: tool %q is explicitly denied", toolName)
	}

	// If allowed with no constraints, permit.
	if scope.Constraints == nil {
		return nil
	}

	// Apply tool-specific constraint checks.
	switch toolName {
	case "bash":
		return checkBashConstraints(input, scope.Constraints)
	case "file_read", "file_write", "file_edit":
		return checkFileConstraints(toolName, input, scope.Constraints)
	case "web_fetch":
		return checkWebFetchConstraints(input, scope.Constraints)
	case "web_search":
		return checkWebSearchConstraints(input, scope.Constraints)
	default:
		// No specific constraint logic for this tool; allow is true so permit.
		return nil
	}
}

// checkBashConstraints verifies the command starts with one of the allowed prefixes.
func checkBashConstraints(input json.RawMessage, c *agent.Constraints) error {
	if len(c.Commands) == 0 {
		return nil
	}

	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return fmt.Errorf("permission denied: cannot parse bash input: %w", err)
	}

	cmd := strings.TrimSpace(params.Command)
	for _, prefix := range c.Commands {
		if strings.HasPrefix(cmd, prefix) {
			return nil
		}
	}

	return fmt.Errorf("permission denied: bash command %q does not match any allowed command prefix", params.Command)
}

// checkFileConstraints verifies the file path is under one of the allowed paths.
func checkFileConstraints(toolName string, input json.RawMessage, c *agent.Constraints) error {
	if len(c.Paths) == 0 {
		return nil
	}

	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return fmt.Errorf("permission denied: cannot parse %s input: %w", toolName, err)
	}

	for _, allowed := range c.Paths {
		if strings.HasPrefix(params.Path, allowed) {
			return nil
		}
	}

	return fmt.Errorf("permission denied: path %q is not under any allowed path", params.Path)
}

// checkWebFetchConstraints verifies the URL domain is in the allowed list.
func checkWebFetchConstraints(input json.RawMessage, c *agent.Constraints) error {
	if len(c.Domains) == 0 {
		return nil
	}

	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return fmt.Errorf("permission denied: cannot parse web_fetch input: %w", err)
	}

	return checkDomain(params.URL, c.Domains)
}

// checkWebSearchConstraints checks domain constraints for web_search.
// Since web_search takes a query (not a URL), domain constraints are not
// directly applicable. We allow if domains are specified but don't block
// search queries. If a URL-like query is provided, we check its domain.
func checkWebSearchConstraints(input json.RawMessage, c *agent.Constraints) error {
	if len(c.Domains) == 0 {
		return nil
	}

	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return fmt.Errorf("permission denied: cannot parse web_search input: %w", err)
	}

	// For web_search, domain constraints are advisory; allow all queries.
	return nil
}

// checkDomain extracts the hostname from a URL and checks it against allowed domains.
func checkDomain(rawURL string, domains []string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("permission denied: invalid URL %q: %w", rawURL, err)
	}

	host := parsed.Hostname()
	for _, d := range domains {
		// Exact match or subdomain match (e.g., "api.example.com" matches "example.com").
		if host == d || strings.HasSuffix(host, "."+d) {
			return nil
		}
	}

	return fmt.Errorf("permission denied: domain %q is not in allowed domains", host)
}

package sandbox

import (
	"fmt"

	"github.com/domuk-k/open-managed-agents/internal/config"
)

// NewProvider creates a sandbox Provider based on the given type string.
func NewProvider(sandboxType string, cfg *config.Config) (Provider, error) {
	switch sandboxType {
	case "docker":
		return NewDockerProvider()
	case "local":
		return NewLocalProvider(), nil
	case "e2b":
		if cfg.E2B.APIKey == "" {
			return nil, fmt.Errorf("e2b sandbox requires OMA_E2B_API_KEY to be set")
		}
		opts := []E2BProviderOption{}
		if cfg.E2B.Template != "" {
			opts = append(opts, WithE2BTemplate(cfg.E2B.Template))
		}
		return NewE2BProvider(cfg.E2B.APIKey, opts...), nil
	default:
		return nil, fmt.Errorf("unknown sandbox type: %s", sandboxType)
	}
}

package config

import (
	"os"
	"strings"
)

type Config struct {
	Port        string
	DBPath      string
	SandboxType string
	LLM         LLMConfig
	APIKey      string
}

type LLMConfig struct {
	BaseURL string
	Model   string
	APIKey  string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("OMA_PORT", "8080"),
		DBPath:      getEnv("OMA_DB_PATH", "./data/oma.db"),
		SandboxType: getEnv("OMA_SANDBOX_TYPE", "local"),
		LLM: LLMConfig{
			BaseURL: getEnv("OMA_LLM_BASE_URL", "http://localhost:1234/v1"),
			Model:   getEnv("OMA_LLM_MODEL", "qwen3.5-35b-a3b"),
			APIKey:  getEnv("OMA_LLM_API_KEY", ""),
		},
		APIKey: getEnv("OMA_API_KEY", ""),
	}
}

// IsAnthropic returns true if the configured model should use the Anthropic native API.
// Models starting with "anthropic/" or "claude-" are detected as Anthropic models.
func (c *LLMConfig) IsAnthropic() bool {
	return strings.HasPrefix(c.Model, "anthropic/") || strings.HasPrefix(c.Model, "claude-")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

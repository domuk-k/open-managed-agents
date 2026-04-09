package environment

import "time"

type Environment struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Config     EnvironmentConfig `json:"config"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ArchivedAt *time.Time        `json:"archived_at,omitempty"`
}

type EnvironmentConfig struct {
	Type       string            `json:"type"`
	Networking NetworkConfig     `json:"networking"`
	Packages   []string          `json:"packages,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
	Resources  *Resources        `json:"resources,omitempty"`
}

type NetworkConfig struct {
	Type           string   `json:"type"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
}

type Resources struct {
	MemoryMB       *int `json:"memory_mb,omitempty"`
	CPUCores       *int `json:"cpu_cores,omitempty"`
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
}

type CreateRequest struct {
	Name   string            `json:"name"`
	Config EnvironmentConfig `json:"config"`
}

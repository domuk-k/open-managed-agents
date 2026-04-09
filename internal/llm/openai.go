package llm

import (
	"net/http"
)

// OpenAIProvider implements the Provider interface using the OpenAI-compatible API.
// This covers OpenAI, LM Studio, Ollama, vLLM, OpenRouter, Together, Groq, etc.
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{},
	}
}

// Chat and Stream implementations will be added in the next phase.

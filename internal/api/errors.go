package api

// apiError returns a structured error response matching the Claude API error format.
func apiError(errType, message string) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	}
}

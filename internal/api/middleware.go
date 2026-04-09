package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// apiKeyAuth returns middleware that validates Bearer token authentication.
// If apiKey is empty, all requests are allowed (no auth configured).
func apiKeyAuth(apiKey string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if apiKey == "" {
				return next(c) // no auth configured
			}
			auth := c.Request().Header.Get("Authorization")
			if auth != "Bearer "+apiKey {
				return c.JSON(http.StatusUnauthorized, apiError("unauthorized", "invalid or missing API key"))
			}
			return next(c)
		}
	}
}

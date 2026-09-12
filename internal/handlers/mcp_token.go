package handlers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
)

type mcpTokenResponse struct {
	Token string `json:"token"`
}

// MintMCPToken issues a long-lived MCP access token for the authenticated user.
// The caller pastes it into their local agent as a bearer token.
func (h *Handlers) MintMCPToken(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.TokenClaims](c)
	token, err := h.mcpAuth.Mint(jwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Could not create MCP token", err, c)
	}
	return c.JSON(http.StatusOK, mcpTokenResponse{Token: token})
}

// RevokeMCPToken invalidates every outstanding MCP token for the authenticated
// user by bumping their MCP auth version.
func (h *Handlers) RevokeMCPToken(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.TokenClaims](c)
	if err := h.db.BumpMCPAuthVersion(c.Request().Context(), jwt.UserId); err != nil {
		if errors.Is(err, mongorepo.UserNotFoundError) {
			return errorResponse(http.StatusUnauthorized, "Invalid or expired token", nil, c)
		}
		return errorResponse(http.StatusInternalServerError, "Could not revoke MCP tokens", err, c)
	}
	return messageResponse(http.StatusOK, "MCP tokens revoked", c)
}

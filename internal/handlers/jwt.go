package handlers

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
	"time"
)

func (h *Handlers) QueryJwt(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		queryToken := c.QueryParam("access_token")
		if queryToken == "" {
			return next(c)
		}

		if headerToken := c.Request().Header.Get("Authorization"); headerToken == "" {
			c.Request().Header.Set("Authorization", fmt.Sprintf("Bearer %s", queryToken))
		}
		return next(c)
	}
}

// @Summary Get a new access token
// @Description Get a new access token by providing a valid refresh token
// @Tags Auth
// @accept json
// @produce json
// @Param request body models.RefreshRequest true "Refresh request"
// @Success 200 {object} models.RefreshResponse "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 500 {object} models.ErrorResponse "Failed to: get the corresponding user / generate a new token"
// @Router /refresh [post]
func (h *Handlers) Refresh(c echo.Context) error {
	var body models.RefreshRequest
	err := c.Bind(&body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	decodedJwt, err := jwt_manager.DecodeJWT[models.RefreshJwt](h.jwt.RefreshSecret, body.RefreshToken, jwt_manager.PurposeRefresh)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, "Invalid or expired refresh token", nil, c)
	}
	if decodedJwt.UserId == "" {
		return errorResponse(http.StatusUnauthorized, "Invalid or expired refresh token", nil, c)
	}

	user, err := h.db.GetUserById(decodedJwt.UserId)
	if err != nil {
		if errors.Is(err, mongorepo.UserNotFoundError) {
			return errorResponse(http.StatusUnauthorized, "Invalid or expired refresh token", nil, c)
		}
		return errorResponse(http.StatusInternalServerError, "Could not refresh access token", err, c)
	}

	accessJwt, accessToken, err := h.jm.GenerateAccessJwt(decodedJwt.UserId, user.Admin, h.jwt.AccessExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Could not generate access token", err, c)
	}

	return c.JSON(http.StatusOK, models.RefreshResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(accessJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:   accessJwt.ExpiresAt.Time,
	})
}

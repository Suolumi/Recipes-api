package handlers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
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

func (h *Handlers) Refresh(c echo.Context) error {
	var body models.RefreshRequest
	err := c.Bind(&body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	decodedJwt, err := jwt_manager.DecodeJWT[models.RefreshJwt](h.jwt.RefreshSecret, body.RefreshToken)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	user, err := h.db.GetUserById(decodedJwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	accessJwt, accessToken, err := h.jm.GenerateAccessJwt(decodedJwt.UserId, user.Admin, h.jwt.AccessExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, models.RefreshResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(accessJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:   accessJwt.ExpiresAt.Time,
	})
}

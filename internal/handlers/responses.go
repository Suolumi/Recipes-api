package handlers

import (
	"errors"
	"github.com/labstack/echo/v4"
	"net/http"
	"recipes/internal/models"
	"recipes/internal/utils"
)

func httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status := http.StatusInternalServerError
	message := "Internal server error"
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.Code
		switch status {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
			http.StatusConflict, http.StatusNotAcceptable, http.StatusTooManyRequests, http.StatusUnprocessableEntity:
			if text, ok := httpErr.Message.(string); ok {
				message = text
			}
		}
	}
	_ = errorResponse(status, message, err, c)
}

// errorResponse is a helper function to return an error response to the client
func errorResponse(code int, message string, err error, c echo.Context) error {
	if err != nil {
		utils.LogError("request failed", err, "path", c.Path(), "status", code, "message", message)
	}
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)
	if requestID == "" {
		requestID = c.Request().Header.Get(echo.HeaderXRequestID)
	}
	return c.JSON(code, models.ErrorResponse{Error: message, Code: errorCode(code), RequestID: requestID})
}

func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusNotAcceptable, http.StatusUnprocessableEntity:
		return "invalid_input"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		return "internal_error"
	}
}

// messageResponse is a helper function to return a message response to the client
func messageResponse(code int, message string, c echo.Context) error {
	return c.JSON(code, models.MessageResponse{Message: message})
}

package handlers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"io"
	"recipes/internal/models"
	"recipes/internal/utils"
)

// errorResponse is a helper function to return an error response to the client
func errorResponse(code int, message string, err error, c echo.Context) error {
	str, e := io.ReadAll(c.Request().Body)
	if e != nil {
		str = []byte("")
	}

	if err != nil {
		utils.LogError(fmt.Sprintf("Path: %s\nBody:\n%s\nMessage: %s", c.Path(), string(str), message), err)
	}
	return c.JSON(code, models.ErrorResponse{Error: message})
}

// messageResponse is a helper function to return a message response to the client
func messageResponse(code int, message string, c echo.Context) error {
	return c.JSON(code, models.MessageResponse{Message: message})
}

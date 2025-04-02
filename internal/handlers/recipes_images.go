package handlers

import (
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
	"os"
	"path/filepath"
	"recipes/internal/images_manager"
	"recipes/internal/models"
)

func (h *Handlers) SaveRecipeImage(c echo.Context) error {
	formFile, err := c.FormFile("file")
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if err := images_manager.CheckImage(formFile); err != nil {
		return errorResponse(http.StatusNotAcceptable, err.Error(), err, c)
	}

	fileName := primitive.NewObjectID().Hex() + filepath.Ext(formFile.Filename)
	err = images_manager.Save(formFile, h.cfg.RecipeImageDir, fileName)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	h.imgLru.Add(fileName, true)
	return c.JSON(http.StatusOK, models.UpdatePictureResponse{Picture: fileName})
}

func (h *Handlers) DeleteRecipeImage(c echo.Context) error {
	userId := c.Param("id")
	user, err := h.db.GetUserById(userId)
	if err != nil {
		return errorResponse(http.StatusNotFound, "User not found", err, c)
	}

	if user.Picture == "" {
		return errorResponse(http.StatusUnprocessableEntity, "No picture for this user", nil, c)
	}

	err = images_manager.Remove(h.cfg.ImagesDir, user.Picture)
	if err != nil && os.IsNotExist(err) {
		return errorResponse(http.StatusNotFound, "Image not found", err, c)
	} else if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return messageResponse(http.StatusOK, "Image removed successfully", c)
}

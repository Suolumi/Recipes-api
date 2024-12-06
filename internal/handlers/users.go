package handlers

import (
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
	"os"
	"path/filepath"
	"recipes/internal/images_manager"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
	"recipes/internal/utils"
)

func (h *Handlers) GetMe(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)

	user, err := h.db.GetUserById(jwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, utils.DupStruct[models.UserMe](&user))
}

func (h *Handlers) UpdateMe(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)

	var user models.UpdateUser
	var err error

	err = c.Bind(&user)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if len(user.Username) > 32 {
		return errorResponse(http.StatusNotAcceptable, "username cannot be more than 32 characters long", nil, c)
	}
	if user.Email != "" && !utils.IsEmailValid(user.Email) {
		return errorResponse(http.StatusNotAcceptable, "Must be a valid email", nil, c)
	}
	if user.Password != "" && !utils.IsPasswordValid(user.Password) {
		return errorResponse(http.StatusNotAcceptable, "Password must contain at least 10 characters, 1 number and 1 special character", nil, c)
	}

	userdb := utils.DupStruct[models.UserDB](&user)
	if conflict, err := h.db.UserConflicts(models.UserDB{
		Username: user.Username,
		Email:    user.Email,
	}); err != nil && conflict.Id.Hex() != jwt.UserId {
		return errorResponse(http.StatusConflict, err.Error(), err, c)
	}

	updatedUser, err := h.db.UpdateUserById(jwt.UserId, userdb)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, utils.DupStruct[models.UserMe](&updatedUser))
}

func (h *Handlers) DeleteMe(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)

	user, err := h.db.DeleteUserById(jwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	err = images_manager.Remove(h.cfg.ImagesDir, user.Picture)
	if err != nil && !os.IsNotExist(err) {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, utils.DupStruct[models.UserMe](&user))
}

func (h *Handlers) UploadProfilePicture(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)

	formFile, err := c.FormFile("file")
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if err := images_manager.CheckImage(formFile); err != nil {
		return errorResponse(http.StatusNotAcceptable, err.Error(), err, c)
	}

	user, err := h.db.GetUserById(jwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	if user.Picture != "" {
		err = images_manager.Remove(h.cfg.ImagesDir, user.Picture)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
		}
	}

	fileName := primitive.NewObjectID().Hex() + filepath.Ext(formFile.Filename)
	_, err = h.db.UpdateUserById(jwt.UserId, models.UserDB{
		Picture: fileName,
	})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	err = images_manager.Save(formFile, h.cfg.ImagesDir, fileName)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, models.UpdatePictureResponse{Picture: fileName})
}

func (h *Handlers) DeleteProfilePicture(c echo.Context) error {
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
	user, err := h.db.GetUserById(jwt.UserId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
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

	_, err = h.db.UpdateUserInterfaceById(jwt.UserId, struct {
		Picture string `bson:"picture"`
	}{})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return messageResponse(http.StatusOK, "Image removed successfully", c)
}

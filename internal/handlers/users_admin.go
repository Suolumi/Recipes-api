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

func (h *Handlers) GetUser(c echo.Context) error {
	userId := c.Param("id")
	admin := false
	if c.Get("jwt") != nil {
		jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
		admin = jwt.Admin

		if jwt.UserId == userId {
			return h.GetMe(c)
		}
	}
	user, err := h.db.GetUserById(userId)
	if err != nil {
		return errorResponse(http.StatusNotFound, "User not found", err, c)
	}

	if err != nil {
		return errorResponse(http.StatusUnauthorized, "invalid or expired jwt", err, c)
	}
	if admin {
		return c.JSON(http.StatusOK, user)
	}
	return c.JSON(http.StatusOK, utils.DupStruct[models.UserView](&user))
}

func (h *Handlers) GetUsers(c echo.Context) error {
	var queryParams models.GetUsersRequest
	err := utils.BindQuery(c, &queryParams)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	// Limit default payload size
	if queryParams.Limit == 0 {
		queryParams.Limit = 10
	}
	users, nbUsers, err := h.db.GetUsers(queryParams.Username, queryParams.Limit, queryParams.Offset)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	if users == nil {
		users = []models.UserDB{}
	}

	admin := false
	if c.Get("jwt") != nil {
		jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
		admin = jwt.Admin
	}
	if admin {
		return c.JSON(http.StatusOK, models.GetUsersResponse{
			Length: nbUsers,
			Items:  users,
		})
	}
	userBasics := make([]models.UserView, len(users))
	for i, u := range users {
		userBasics[i] = utils.DupStruct[models.UserView](&u)
	}
	return c.JSON(http.StatusOK, models.GetUsersResponse{
		Length: nbUsers,
		Items:  userBasics,
	})
}

func (h *Handlers) UpdateUser(c echo.Context) error {
	userId := c.Param("id")
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
	var userData models.UserDB

	err := c.Bind(&userData)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if len(userData.Username) > 32 {
		return errorResponse(http.StatusBadRequest, "username cannot be more than 32 characters long", nil, c)
	}

	if conflict, err := h.db.UserConflicts(models.UserDB{
		Username: userData.Username,
		Email:    userData.Email,
	}); err != nil && conflict.Id.Hex() != jwt.UserId {
		return errorResponse(http.StatusConflict, err.Error(), err, c)
	}

	updatedUser, err := h.db.UpdateUserById(userId, userData)
	if err != nil {
		return errorResponse(http.StatusNotFound, "User not found", err, c)
	}

	return c.JSON(http.StatusOK, updatedUser)
}

func (h *Handlers) DeleteUser(c echo.Context) error {
	userId := c.Param("id")

	user, err := h.db.DeleteUserById(userId)
	if err != nil {
		return errorResponse(http.StatusNotFound, "User not found", err, c)
	}

	if user.Picture != "" {
		err = images_manager.Remove(h.cfg.ImagesDir, user.Picture)
	}
	if err != nil && !os.IsNotExist(err) {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *Handlers) UpdateUserPicture(c echo.Context) error {
	userId := c.Param("id")

	formFile, err := c.FormFile("file")
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if err := images_manager.CheckImage(formFile); err != nil {
		return errorResponse(http.StatusNotAcceptable, err.Error(), err, c)
	}

	user, err := h.db.GetUserById(userId)
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
	_, err = h.db.UpdateUserById(userId, models.UserDB{
		Picture: fileName,
	})
	if err != nil {
		return errorResponse(http.StatusNotFound, "User not found", err, c)
	}

	err = images_manager.Save(formFile, h.cfg.ImagesDir, fileName)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, models.UpdatePictureResponse{Picture: fileName})
}

func (h *Handlers) DeleteUserPicture(c echo.Context) error {
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

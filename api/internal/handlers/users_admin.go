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

// @Summary Get user by id
// @Description Get another user by its id
// @Tags Users
// @produce json
// @Param id path string true "User id to get"
// @Success 200 {object} models.UserView "OK"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Security BearerAuth
// @Router /users/{id} [get]
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

	if admin {
		return c.JSON(http.StatusOK, user)
	}
	return c.JSON(http.StatusOK, utils.DupStruct[models.UserView](&user))
}

// @Summary Get users
// @Description Get users with filters
// @Tags Users
// @produce json
// @Param request query models.GetUsersRequest true "Query parameters"
// @Success 200 {object} models.GetUsersResponse "OK"
// @Failure 400 {object} models.ErrorResponse "Couldn't read query parameters"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 500 {object} models.ErrorResponse "Failed to get users"
// @Security BearerAuth
// @Router /users [get]
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

// @Summary Update user
// @Description Update another user by its id. Must be admin
// @Tags Users
// @accept json
// @produce json
// @Param id path string true "User id to update" example(6567bd0a84f663dbe91176f5)
// @Param request body models.UserDB true "Infos to update"
// @Success 200 {object} models.UserDB "OK"
// @Failure 400 {object} models.ErrorResponse "Bad request / Username too long"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 403 {object} models.ErrorResponse "Unknown / Insufficient permissions"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Failure 409 {object} models.ErrorResponse "Username or email already taken"
// @Security BearerAuth
// @Router /users/{id} [patch]
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

// @Summary Delete user
// @Description Delete a user. Must be admin
// @Tags Users
// @produce json
// @Param id path string true "User id to delete" example(6567bd0a84f663dbe91176f5)
// @Success 200 {object} models.UserDB "User deleted"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 403 {object} models.ErrorResponse "Unknown / Insufficient permissions"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Failure 500 {object} models.ErrorResponse "Failed to delete the user / delete associated picture / delete parameters"
// @Security BearerAuth
// @Router /users/{id} [delete]
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

// @Summary Update user picture
// @Description Update a user's profile picture, must be under 8MB. Must be admin
// @Tags Users
// @accept mpfd
// @produce json
// @Param id path string true "User id to update the picture" example(6567bd0a84f663dbe91176f5)
// @Param request formData file true "Picture, formData must be named 'file'"
// @Success 200 {object} models.UpdatePictureResponse "OK"
// @Failure 400 {object} models.ErrorResponse "Empty formData / not named correctly"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 403 {object} models.ErrorResponse "Unknown / Insufficient permissions"
// @Failure 406 {object} models.ErrorResponse "Wrong format (not png / jpeg / jpg) / File too heavy"
// @Failure 500 {object} models.ErrorResponse "Failed to: Update user, save the image"
// @Security BearerAuth
// @Router /users/{id}/picture [post]
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
	return c.JSON(http.StatusOK, models.UpdatePictureResponse{Id: fileName})
}

// @Summary Delete user picture
// @Description Delete a user's profile picture. Must be admin
// @Tags Users
// @produce json
// @Param id path string true "User id to delete the picture from" example(6567bd0a84f663dbe91176f5)
// @Success 200 {object} models.MessageResponse "Picture deleted"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired token"
// @Failure 403 {object} models.ErrorResponse "Unknown / Insufficient permissions"
// @Failure 404 {object} models.ErrorResponse "Image not found"
// @Failure 422 {object} models.ErrorResponse "No picture for this user"
// @Failure 500 {object} models.ErrorResponse "Failed to: Update user / remove the picture"
// @Security BearerAuth
// @Router /users/{id}/picture [delete]
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

// @Summary Get profile picture
// @Description Get a profile picture
// @Tags Users
// @produce jpeg
// @produce png
// @Param id path string true "Id of the picture with the extension" example(6567bd0a84f663dbe91176f5.png)
// @Success 200 "Jpeg, png or jpg image"
// @Failure 404 "Not found"
// @Router /pictures/{id} [get]
func _() {}

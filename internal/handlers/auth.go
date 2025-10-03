package handlers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
	"recipes/internal/utils"
	"time"
)

// @Summary Login
// @Description Log in with email/username and password to get access and refresh tokens
// @Tags Auth
// @accept json
// @produce json
// @Param request body models.LoginRequest true "Login request"
// @Success 200 {object} models.LoginResponse "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 404 {object} models.ErrorResponse "Incorrect username or password"
// @Failure 500 {object} models.ErrorResponse "Failed to generate tokens"
// @Router /login [post]
func (h *Handlers) Login(c echo.Context) error {
	var reqUser models.LoginRequest
	err := c.Bind(&reqUser)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	user, err := h.db.UserConflicts(models.UserDB{
		Username: reqUser.Identifier,
		Email:    reqUser.Identifier,
	})
	if err == nil {
		return errorResponse(http.StatusNotFound, "Incorrect username or password", nil, c)
	}

	if err = utils.ComparePasswords(user.Password, reqUser.Password); err != nil {
		return errorResponse(http.StatusNotFound, "Incorrect username or password", nil, c)
	}

	accessJwt, accessToken, err := h.jm.GenerateAccessJwt(user.Id.Hex(), user.Admin, h.jwt.AccessExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	refreshJwt, refreshToken, err := h.jm.GenerateRefreshJwt(user.Id.Hex(), user.Admin, h.jwt.RefreshExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, models.LoginResponse{
		AccessToken:           accessToken,
		ExpiresIn:             int(accessJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:             accessJwt.ExpiresAt.Time,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresIn: int(refreshJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		RefreshTokenExpiresAt: refreshJwt.ExpiresAt.Time,
	})
}

// @Summary Register
// @Description Create an account with needed information
// @Tags Auth
// @accept json
// @produce json
// @Param request body models.RegisterRequest true "Register request"
// @Success 201 {object} models.RegisterResponse "User created"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 406 {object} models.ErrorResponse "Password not strong enough / Invalid mail / Invalid username"
// @Failure 409 {object} models.ErrorResponse "User already exists"
// @Failure 500 {object} models.ErrorResponse "Failed to: generate username / check if username was banned / create user / create jwt / send mail"
// @Router /register [post]
func (h *Handlers) Register(c echo.Context) error {
	var user models.RegisterRequest

	err := c.Bind(&user)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}
	// Check if required fields are present and valid
	if user.Username == "" {
		return errorResponse(http.StatusBadRequest, "username is empty", nil, c)
	}
	if len(user.Username) > 32 {
		return errorResponse(http.StatusBadRequest, "username cannot be more than 32 characters long", nil, c)
	}
	if !utils.IsEmailValid(user.Email) {
		return errorResponse(http.StatusNotAcceptable, "Must be a valid email", nil, c)
	}
	if !utils.IsPasswordValid(user.Password) {
		return errorResponse(http.StatusNotAcceptable, "Password must contain at least 10 characters, 1 number and 1 special character", nil, c)
	}

	// Check if user already exists
	if _, err := h.db.UserConflicts(models.UserDB{
		Username: user.Username,
		Email:    user.Email,
	}); err != nil {
		return errorResponse(http.StatusConflict, err.Error(), err, c)
	}

	// Create user
	createdUser, err := h.db.CreateUser(models.UserDB{
		Username: user.Username,
		Email:    user.Email,
		Password: user.Password,
	})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	accessJwt, accessToken, err := h.jm.GenerateAccessJwt(createdUser.Id.Hex(), createdUser.Admin, h.jwt.AccessExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	refreshJwt, refreshToken, err := h.jm.GenerateRefreshJwt(createdUser.Id.Hex(), createdUser.Admin, h.jwt.RefreshExpiration)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	return c.JSON(http.StatusOK, models.RegisterResponse{
		AccessToken:           accessToken,
		ExpiresIn:             int(accessJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:             accessJwt.ExpiresAt.Time,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresIn: int(refreshJwt.ExpiresAt.Sub(time.Now()).Seconds()),
		RefreshTokenExpiresAt: refreshJwt.ExpiresAt.Time,
	})
}

func (h *Handlers) ForgotPassword(c echo.Context) error {
	var data models.ForgotPasswordRequest

	err := c.Bind(&data)
	if err != nil || data.Identifier == "" {
		return errorResponse(http.StatusBadRequest, "Missing informations", err, c)
	}

	user, err := h.db.UserConflicts(models.UserDB{
		Email:    data.Identifier,
		Username: data.Identifier,
	})
	if err == nil {
		return errorResponse(http.StatusNotFound, "User not found", nil, c)
	}

	_, token, err := h.jm.GenerateResetJwt(user.Id.Hex(), time.Hour*24*7)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	if data.Locale == "" {
		data.Locale = "en"
	}
	err = h.ms.SendMail("Password reinitialization",
		[]string{user.Email},
		h.ms.ResetPasswordMail, map[string]string{
			"USER_NAME":  user.Username,
			"RESET_LINK": fmt.Sprintf("https://recipes.suolumi.fr/%s/forgot-password/%s", data.Locale, token),
			"USER_EMAIL": user.Email,
		})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Could not send the verification mail", err, c)
	}

	return messageResponse(http.StatusOK, "Email sent", c)
}

func (h *Handlers) ResetPassword(c echo.Context) error {
	token := c.Param("token")
	var data models.ResetPasswordRequest

	jwt, err := jwt_manager.DecodeJWT[models.ResetJwt](h.jm.ResetSecret, token)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, err.Error(), err, c)
	}

	if jwt.ExpiresAt == nil || jwt.ExpiresAt.Before(time.Now()) {
		return errorResponse(http.StatusUnauthorized, "Token expired", nil, c)
	}

	err = c.Bind(&data)
	if err != nil || !utils.IsPasswordValid(data.Password) {
		return errorResponse(http.StatusNotAcceptable, "Password must contain at least 10 characters, 1 number and 1 special character", err, c)
	}

	if _, err = h.db.UpdateUserById(jwt.UserId, models.UserDB{
		Password: data.Password,
	}); err != nil {
		return errorResponse(http.StatusInternalServerError, "Could not update the password", err, c)
	}

	return messageResponse(http.StatusOK, "Password reset", c)
}

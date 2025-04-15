package handlers

import (
	"github.com/labstack/echo/v4"
	"net/http"
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

// @Summary Create an account
// @Description Create an account with needed information
// @Tags Auth
// @accept json
// @produce json
// @Param request body models.RegisterRequest true "Register request"
// @Success 201 {object} models.MessageResponse "User created"
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
	_, err = h.db.CreateUser(models.UserDB{
		Username: user.Username,
		Email:    user.Email,
		Password: user.Password,
	})
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}

	//_, token, err := h.jm.GenerateRegisterJwt(id, time.Hour*24*7)
	//if err != nil {
	//	return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	//}
	//
	//err = h.ms.SendMail("Log in with Echo",
	//	[]string{user.Email},
	//	h.ms.RegisterMail, map[string]string{
	//		"username":     user.Username,
	//		"redirect_url": "https://app.echo-app.fr/validate/" + token,
	//	})
	//if err != nil {
	//	return errorResponse(http.StatusInternalServerError, "Could not send the verification mail", err, c)
	//}

	return messageResponse(http.StatusCreated, "User created", c)
}

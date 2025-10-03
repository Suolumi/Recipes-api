package models

import (
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type AccessJwt struct {
	UserId string `json:"id"`
	Admin  bool   `json:"admin"`
	jwt.RegisteredClaims
}

type RefreshJwt struct {
	UserId string `json:"id"`
	Admin  bool   `json:"admin"`
	jwt.RegisteredClaims
}

type ResetJwt struct {
	UserId string `json:"id"`
	jwt.RegisteredClaims
}

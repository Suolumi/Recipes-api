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

// TokenClaims is the payload of every JWT the API issues (access, refresh,
// reset). The Purpose field distinguishes them and is checked on decode.
type TokenClaims struct {
	UserId  string `json:"id"`
	Admin   bool   `json:"admin"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

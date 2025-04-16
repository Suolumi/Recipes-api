package models

import "time"

type LoginRequest struct {
	Identifier string `json:"id"`
	Password   string `json:"password"`
}

type LoginResponse struct {
	AccessToken           string    `json:"access_token"`
	ExpiresIn             int       `json:"expires_in"`
	ExpiresAt             time.Time `json:"expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresIn int       `json:"refresh_token_expires_in"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

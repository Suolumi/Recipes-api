package models

type ErrorResponse struct {
	Error string `json:"error,omitempty"`
}

type MessageResponse struct {
	Message string `json:"message,omitempty"`
}

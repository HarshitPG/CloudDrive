package rest

// Common Response Models shared across all REST handlers
type errorResponse struct {
	Error string `json:"error" example:"invalid request"`
}

type messageResponse struct {
	Message string `json:"message" example:"operation completed successfully"`
}
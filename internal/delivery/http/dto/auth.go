package dto

import (
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type RegisterRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=100" example:"Harun"`
	Email    string `json:"email" binding:"required,email,max=254" example:"harun@example.com"`
	Password string `json:"password" binding:"required,min=8,max=72" example:"s3cretpass"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email" example:"harun@example.com"`
	Password string `json:"password" binding:"required" example:"s3cretpass"`
}

type TokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type" example:"Bearer"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role" example:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func NewUserResponse(u *domain.User) UserResponse {
	return UserResponse{ID: u.ID, Name: u.Name, Email: u.Email, Role: string(u.Role), CreatedAt: u.CreatedAt}
}

// Envelope types below exist only so swag can document the response shape.

type UserEnvelope struct {
	Data UserResponse `json:"data"`
}

type TokenEnvelope struct {
	Data TokenResponse `json:"data"`
}

type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

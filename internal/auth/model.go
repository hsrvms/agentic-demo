package auth

import (
	"context"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims are embedded in the JWT.
type Claims struct {
	jwt.RegisteredClaims
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

// AuthService is the module interface.
type AuthService interface {
	Register(ctx context.Context, email, password string) (domain.User, error)
	Login(ctx context.Context, email, password string) (string, error)
	ValidateToken(ctx context.Context, tokenString string) (*domain.AuthClaims, error)
}

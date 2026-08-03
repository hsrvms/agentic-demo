package auth

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrUserExists, http.StatusConflict, "user already exists")
	httperr.Register(ErrInvalidCredentials, http.StatusUnauthorized, "invalid credentials")
	httperr.Register(ErrInvalidEmail, http.StatusBadRequest, "invalid email address")
	httperr.Register(ErrWeakPassword, http.StatusBadRequest, "password must be at least 8 characters")
	httperr.Register(ErrUserNotFound, http.StatusNotFound, "user not found")
	httperr.Register(ErrInvalidToken, http.StatusUnauthorized, "invalid or expired token")
}
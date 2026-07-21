package auth

import "errors"

var (
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

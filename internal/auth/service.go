package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultBcryptCost = 10
	tokenExpiry       = 24 * time.Hour
	minPassword       = 8
)

type authService struct {
	repo       Repository
	jwtSecret  []byte
	bcryptCost int
}

// NewService creates an AuthService.
func NewService(repo Repository, jwtSecret []byte) AuthService {
	return &authService{
		repo:       repo,
		jwtSecret:  jwtSecret,
		bcryptCost: defaultBcryptCost,
	}
}

func (s *authService) Register(ctx context.Context, email, password string) (domain.User, error) {
	// Validate email.
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return domain.User{}, ErrInvalidEmail
	}

	// Validate password.
	if len(password) < minPassword {
		return domain.User{}, ErrWeakPassword
	}

	// Check for existing user.
	_, err := s.repo.GetUserByEmail(ctx, email)
	if err == nil {
		return domain.User{}, ErrUserExists
	}
	if !errors.Is(err, ErrUserNotFound) {
		return domain.User{}, err
	}

	// Hash password.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return domain.User{}, err
	}

	return s.repo.CreateUser(ctx, email, hash)
}

func (s *authService) Login(ctx context.Context, email, password string) (string, error) {
	email = strings.TrimSpace(email)

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	return s.issueToken(user.ID, user.Email)
}

func (s *authService) ValidateToken(_ context.Context, tokenString string) (*domain.AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return &domain.AuthClaims{
		UserID: claims.UserID,
		Email:  claims.Email,
	}, nil
}

func (s *authService) issueToken(userID uuid.UUID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID.String(),
		},
		UserID: userID,
		Email:  email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

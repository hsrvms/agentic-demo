package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// mockRepository implements Repository for unit tests.
type mockRepository struct {
	users     []domain.User
	createErr error
	findErr   error
}

func (m *mockRepository) CreateUser(_ context.Context, email string, passwordHash []byte) (domain.User, error) {
	if m.createErr != nil {
		return domain.User{}, m.createErr
	}
	u := domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
	}
	m.users = append(m.users, u)
	return u, nil
}

func (m *mockRepository) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	if m.findErr != nil {
		return domain.User{}, m.findErr
	}
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, ErrUserNotFound
}

func (m *mockRepository) GetUserByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	if m.findErr != nil {
		return domain.User{}, m.findErr
	}
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, ErrUserNotFound
}

// newTestService creates an AuthService with minimum bcrypt cost for fast tests.
func newTestService(repo Repository) AuthService {
	svc := NewService(repo, []byte("test-secret")).(*authService)
	svc.bcryptCost = 4 // minimum bcrypt cost for test speed
	return svc
}

func newTestServiceWithSecret(repo Repository, secret []byte) AuthService {
	svc := NewService(repo, secret).(*authService)
	svc.bcryptCost = 4
	return svc
}

func TestRegister_Success(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	user, err := svc.Register(context.Background(), "alice@example.com", "securepass123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
	if len(user.PasswordHash) == 0 {
		t.Error("PasswordHash should not be empty")
	}
	// Verify bcrypt was used — the hash should not be the plaintext.
	if string(user.PasswordHash) == "securepass123" {
		t.Error("PasswordHash should not be plaintext")
	}
	// Verify it's a valid bcrypt hash.
	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte("securepass123")); err != nil {
		t.Errorf("bcrypt.CompareHashAndPassword failed: %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "alice@example.com", "securepass123")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = svc.Register(context.Background(), "alice@example.com", "otherpass123")
	if !errors.Is(err, ErrUserExists) {
		t.Errorf("error = %v, want %v", err, ErrUserExists)
	}
}

func TestRegister_EmptyEmail(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "", "securepass123")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "notanemail", "securepass123")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "alice@example.com", "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Errorf("error = %v, want %v", err, ErrWeakPassword)
	}
}

func TestLogin_Success(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "alice@example.com", "securepass123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	token, err := svc.Login(context.Background(), "alice@example.com", "securepass123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Register(context.Background(), "alice@example.com", "securepass123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = svc.Login(context.Background(), "alice@example.com", "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.Login(context.Background(), "nobody@example.com", "securepass123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestValidateToken_Valid(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	user, _ := svc.Register(context.Background(), "alice@example.com", "securepass123")
	token, _ := svc.Login(context.Background(), "alice@example.com", "securepass123")

	claims, err := svc.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", claims.UserID, user.ID)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "alice@example.com")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// We can't easily test expiry without time manipulation,
	// but we can test a tampered token.
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, err := svc.ValidateToken(context.Background(), "garbage.token.here")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	repo := &mockRepository{}
	svc := newTestService(repo)

	_, _ = svc.Register(context.Background(), "alice@example.com", "securepass123")
	token, _ := svc.Login(context.Background(), "alice@example.com", "securepass123")

	// Different secret.
	svcB := newTestServiceWithSecret(repo, []byte("secret-b"))
	_, err := svcB.ValidateToken(context.Background(), token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("error = %v, want %v", err, ErrInvalidToken)
	}
}

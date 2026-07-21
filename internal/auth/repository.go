package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

// Repository abstracts user data access.
type Repository interface {
	CreateUser(ctx context.Context, email string, passwordHash []byte) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (domain.User, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

// NewRepository creates a Repository backed by Postgres.
func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) CreateUser(ctx context.Context, email string, passwordHash []byte) (domain.User, error) {
	row, err := r.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return toDomainUser(row), nil
}

func (r *pgRepository) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return domain.User{}, err
		}
		// pgx returns ErrNoRows when not found.
		return domain.User{}, ErrUserNotFound
	}
	return toDomainUser(row), nil
}

func (r *pgRepository) GetUserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return domain.User{}, err
		}
		return domain.User{}, ErrUserNotFound
	}
	return toDomainUser(row), nil
}

func toDomainUser(row db.User) domain.User {
	return domain.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

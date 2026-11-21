package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database/sqlc"
)

type UserRepository struct {
	q *sqlc.Queries
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{q: sqlc.New(pool)}
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
	})
	if err != nil {
		// The insert selects the role by title, so no rows means the role does not exist.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("create user: unknown role %q", u.Role)
		}
		if pgCode(err) == pgerrcode.UniqueViolation {
			return nil, domain.ErrEmailTaken
		}
		return nil, mapErr(err, "create user")
	}
	return toDomain(row, string(u.Role)), nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, mapErr(err, "get user by id")
	}
	return toDomain(row.User, row.Role), nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, mapErr(err, "get user by email")
	}
	return toDomain(row.User, row.Role), nil
}

func toDomain(row sqlc.User, role string) *domain.User {
	return &domain.User{
		ID:           row.ID,
		Role:         domain.Role(role),
		Name:         row.Name,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

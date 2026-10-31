package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/moni-be/internal/domain"
	"github.com/Flawed-Four/moni-be/internal/infrastructure/database/sqlc"
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
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, domain.ErrEmailTaken
		}
		return nil, err
	}
	return toDomain(row), nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomain(row), nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomain(row), nil
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	// An id that is not a valid UUID can never match a row.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InvalidTextRepresentation {
		return domain.ErrNotFound
	}
	return err
}

func toDomain(row sqlc.User) *domain.User {
	return &domain.User{
		ID:           row.ID,
		Name:         row.Name,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

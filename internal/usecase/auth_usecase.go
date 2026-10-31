package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type AuthUsecase struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
	tokens domain.TokenService
}

func NewAuthUsecase(users domain.UserRepository, hasher domain.PasswordHasher, tokens domain.TokenService) *AuthUsecase {
	return &AuthUsecase{users: users, hasher: hasher, tokens: tokens}
}

func (u *AuthUsecase) Register(ctx context.Context, name, email, password string) (*domain.User, error) {
	email = normalizeEmail(email)

	hash, err := u.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	return u.users.Create(ctx, &domain.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
	})
}

// Login returns an access token for valid credentials. Unknown email and
// wrong password both yield ErrInvalidCredentials to avoid user enumeration.
func (u *AuthUsecase) Login(ctx context.Context, email, password string) (string, error) {
	user, err := u.users.GetByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrInvalidCredentials
		}
		return "", err
	}

	if err := u.hasher.Compare(user.PasswordHash, password); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	token, err := u.tokens.Generate(user.ID)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return token, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

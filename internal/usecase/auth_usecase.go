package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

// SessionTTL is how long a session stays valid after login.
type SessionTTL time.Duration

// ClientInfo describes the client a session is created for.
type ClientInfo struct {
	UserAgent string
	IP        string
}

// Session is the result of a successful login.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

type AuthUsecase struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	hasher   domain.PasswordHasher
	tokens   domain.TokenService
	ttl      time.Duration
}

func NewAuthUsecase(
	users domain.UserRepository,
	sessions domain.SessionRepository,
	hasher domain.PasswordHasher,
	tokens domain.TokenService,
	ttl SessionTTL,
) *AuthUsecase {
	return &AuthUsecase{users: users, sessions: sessions, hasher: hasher, tokens: tokens, ttl: time.Duration(ttl)}
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

// Login creates a server-side session for valid credentials. Unknown email and
// wrong password both yield ErrInvalidCredentials to avoid user enumeration.
func (u *AuthUsecase) Login(ctx context.Context, email, password string, client ClientInfo) (*Session, error) {
	user, err := u.users.GetByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := u.hasher.Compare(user.PasswordHash, password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	token, hash, err := u.tokens.New()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	created, err := u.sessions.Create(ctx, &domain.Session{
		UserID:    user.ID,
		TokenHash: hash,
		UserAgent: client.UserAgent,
		IPAddress: client.IP,
		ExpiresAt: time.Now().Add(u.ttl),
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{Token: token, ExpiresAt: created.ExpiresAt}, nil
}

// Authenticate resolves a token to its user ID, or ErrInvalidToken if the
// session is unknown, revoked or expired.
func (u *AuthUsecase) Authenticate(ctx context.Context, token string) (string, error) {
	s, err := u.sessions.Touch(ctx, u.tokens.Hash(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrInvalidToken
		}
		return "", err
	}
	return s.UserID, nil
}

// Logout revokes the session behind token.
func (u *AuthUsecase) Logout(ctx context.Context, token string) error {
	return u.sessions.Revoke(ctx, u.tokens.Hash(token))
}

// LogoutAll revokes every session of the user.
func (u *AuthUsecase) LogoutAll(ctx context.Context, userID string) error {
	return u.sessions.RevokeAllForUser(ctx, userID)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

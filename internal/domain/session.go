package domain

import (
	"context"
	"time"
)

type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	UserAgent  string
	IPAddress  string // empty when unknown
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt time.Time
	CreatedAt  time.Time
}

// SessionRepository stores server-side sessions keyed by token hash.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) (*Session, error)
	// Touch returns the session for tokenHash and bumps last_used_at, but only
	// if it is neither revoked nor expired. Otherwise it returns ErrNotFound.
	Touch(ctx context.Context, tokenHash []byte) (*Session, error)
	// Revoke is idempotent: revoking an unknown or already revoked session is not an error.
	Revoke(ctx context.Context, tokenHash []byte) error
	RevokeAllForUser(ctx context.Context, userID string) error
}

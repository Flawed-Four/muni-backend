package postgres

import (
	"context"
	"net/netip"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database/sqlc"
)

type SessionRepository struct {
	q *sqlc.Queries
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{q: sqlc.New(pool)}
}

func (r *SessionRepository) Create(ctx context.Context, s *domain.Session) (*domain.Session, error) {
	var ip *netip.Addr
	if addr, err := netip.ParseAddr(s.IPAddress); err == nil {
		ip = &addr
	}

	row, err := r.q.CreateSession(ctx, sqlc.CreateSessionParams{
		UserID:    s.UserID,
		TokenHash: s.TokenHash,
		UserAgent: s.UserAgent,
		IpAddress: ip,
		ExpiresAt: pgtype.Timestamptz{Time: s.ExpiresAt, Valid: true},
	})
	if err != nil {
		return nil, mapErr(err, "create session")
	}
	return sessionToDomain(row), nil
}

func (r *SessionRepository) Touch(ctx context.Context, tokenHash []byte) (*domain.Session, error) {
	row, err := r.q.TouchActiveSession(ctx, tokenHash)
	if err != nil {
		return nil, mapErr(err, "touch session")
	}
	return sessionToDomain(row), nil
}

func (r *SessionRepository) Revoke(ctx context.Context, tokenHash []byte) error {
	_, err := r.q.RevokeSession(ctx, tokenHash)
	return mapErr(err, "revoke session")
}

func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	return mapErr(r.q.RevokeUserSessions(ctx, userID), "revoke user sessions")
}

func sessionToDomain(row sqlc.Session) *domain.Session {
	s := &domain.Session{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		UserAgent:  row.UserAgent,
		ExpiresAt:  row.ExpiresAt.Time,
		LastUsedAt: row.LastUsedAt.Time,
		CreatedAt:  row.CreatedAt.Time,
	}
	if row.IpAddress != nil {
		s.IPAddress = row.IpAddress.String()
	}
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		s.RevokedAt = &t
	}
	return s
}

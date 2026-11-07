package domain

import (
	"context"
	"time"
)

// KeyValueStore holds short-lived data shared between app instances, such as
// cache entries, one-time codes and rate-limit counters. Every key expires, so
// writes reject a TTL that is not positive. Callers own the key names and
// namespace them by feature (for example "otp:login:<user id>").
type KeyValueStore interface {
	// Get returns ErrNotFound when key is absent or expired.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set writes value, replacing any existing value and TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// SetNX writes value only if key is absent and reports whether it did. An
	// existing key keeps its value and TTL. Of several concurrent callers exactly
	// one wins, which suits resend cooldowns and locks.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	// GetDel atomically returns and removes key, so a one-time code can only be
	// consumed once. It returns ErrNotFound when key is absent or expired.
	GetDel(ctx context.Context, key string) ([]byte, error)
	// Delete is idempotent: deleting an absent key is not an error.
	Delete(ctx context.Context, key string) error
	// Incr atomically adds 1 to a counter and returns the new count. ttl only
	// applies when the call creates the counter, so it resets a fixed time after
	// the first hit instead of sliding forward. That makes it a fixed-window rate
	// limit or a cap on OTP attempts.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

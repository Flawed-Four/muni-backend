// Package kvstore keeps short-lived shared data in Redis.
package kvstore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

// NewRedisClient opens a connection pool and verifies connectivity.
// The returned cleanup func closes the pool.
func NewRedisClient(cfg *config.Config) (*redis.Client, func(), error) {
	c := cfg.Redis

	// go-redis reads a zero timeout as "use the default" and the ping context
	// below would expire at once, so reject it up front.
	if c.Timeout <= 0 {
		return nil, nil, fmt.Errorf("redis timeout must be positive, got %s", c.Timeout)
	}

	opts := &redis.Options{
		Addr:         net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Username:     c.Username,
		Password:     c.Password,
		DB:           c.DB,
		DialTimeout:  c.Timeout,
		ReadTimeout:  c.Timeout,
		WriteTimeout: c.Timeout,
		// Off by default, which would let a stalled server hold a request past
		// the deadline of its context.
		ContextTimeoutEnabled: true,
	}
	if c.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host}
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("ping redis at %s: %w", opts.Addr, err)
	}
	return client, func() { _ = client.Close() }, nil
}

// RedisStore implements domain.KeyValueStore. It can be shared between goroutines.
type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := s.client.Get(ctx, key).Bytes()
	return readResult("get", value, err)
}

func (s *RedisStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := checkTTL(ttl); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	if err := s.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (s *RedisStore) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if err := checkTTL(ttl); err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}
	written, err := s.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}
	return written, nil
}

func (s *RedisStore) GetDel(ctx context.Context, key string) ([]byte, error) {
	value, err := s.client.GetDel(ctx, key).Bytes()
	return readResult("getdel", value, err)
}

func (s *RedisStore) Delete(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Incr creates the counter with its TTL and then increments it. The transaction
// keeps the two together: the counter could otherwise expire between them, and
// INCR would recreate it without a TTL, locking its subject out for good.
func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if err := checkTTL(ttl); err != nil {
		return 0, fmt.Errorf("redis incr: %w", err)
	}

	var count *redis.IntCmd
	_, err := s.client.TxPipelined(ctx, func(tx redis.Pipeliner) error {
		tx.SetNX(ctx, key, 0, ttl)
		count = tx.Incr(ctx, key)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("redis incr: %w", err)
	}
	return count.Val(), nil
}

// checkTTL guards the rule that every key expires. go-redis would otherwise
// read 0 as "never expire" and -1 as "keep the current TTL".
func checkTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("ttl must be positive, got %s", ttl)
	}
	return nil
}

func readResult(op string, value []byte, err error) ([]byte, error) {
	switch {
	case errors.Is(err, redis.Nil):
		return nil, domain.ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("redis %s: %w", op, err)
	}
	return value, nil
}

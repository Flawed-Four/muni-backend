package integration

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/kvstore"
)

func newStore(t *testing.T) *kvstore.RedisStore {
	t.Helper()
	resetRedis(t)
	return kvstore.NewRedisStore(testRedis)
}

// ttlOf reads the remaining TTL straight from Redis. Redis reports a key without
// an expiry as -1 and an absent key as -2.
func ttlOf(t *testing.T, key string) time.Duration {
	t.Helper()
	ttl, err := testRedis.PTTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("PTTL %s: %v", key, err)
	}
	return ttl
}

func TestRedisStore_SetAndGet(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Not valid UTF-8, so a string conversion on the way would corrupt it.
	value := []byte{0x00, 0xff, 0xfe, '\r', '\n', 0x00}
	if err := store.Set(ctx, "cache:bin", value, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get(ctx, "cache:bin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("Get = %x, want %x", got, value)
	}
	if ttl := ttlOf(t, "cache:bin"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s, want (0, 1m]", ttl)
	}
}

func TestRedisStore_SetReplacesValueAndTTL(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "cache:k", []byte("old"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(ctx, "cache:k", []byte("new"), time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get(ctx, "cache:k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("Get = %q, want new", got)
	}
	if ttl := ttlOf(t, "cache:k"); ttl <= time.Minute {
		t.Fatalf("TTL = %s, want the replaced TTL of 1h", ttl)
	}
}

// An empty value is a hit. Mistaking it for a miss would make a cached empty
// result look absent.
func TestRedisStore_EmptyValueIsNotAMiss(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "cache:empty", nil, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(ctx, "cache:empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Get = %q, want an empty value", got)
	}
}

func TestRedisStore_AbsentKeyIsNotFound(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if _, err := store.Get(ctx, "cache:absent"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetDel(ctx, "cache:absent"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetDel error = %v, want ErrNotFound", err)
	}
}

func TestRedisStore_KeyExpires(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "cache:short", []byte("v"), 50*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := store.Get(ctx, "cache:short"); err != nil {
		t.Fatalf("Get before expiry: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if _, err := store.Get(ctx, "cache:short"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after expiry error = %v, want ErrNotFound", err)
	}
}

func TestRedisStore_RejectsTTLThatWouldNotExpire(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	writes := []struct {
		name  string
		write func(ttl time.Duration) error
	}{
		{"Set", func(ttl time.Duration) error { return store.Set(ctx, "k", []byte("v"), ttl) }},
		{"SetNX", func(ttl time.Duration) error { _, err := store.SetNX(ctx, "k", []byte("v"), ttl); return err }},
		{"Incr", func(ttl time.Duration) error { _, err := store.Incr(ctx, "k", ttl); return err }},
	}

	for _, w := range writes {
		for _, ttl := range []time.Duration{0, -1, -time.Hour} {
			t.Run(w.name+"/"+ttl.String(), func(t *testing.T) {
				if err := w.write(ttl); err == nil {
					t.Fatal("write succeeded, want an error")
				}
				if got := ttlOf(t, "k"); got != -2 {
					t.Fatalf("key exists with TTL %s, want it absent", got)
				}
			})
		}
	}
}

func TestRedisStore_SetNXKeepsExistingKey(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	written, err := store.SetNX(ctx, "otp:cooldown", []byte("first"), time.Minute)
	if err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	if !written {
		t.Fatal("first SetNX did not write")
	}

	written, err = store.SetNX(ctx, "otp:cooldown", []byte("second"), time.Hour)
	if err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	if written {
		t.Fatal("second SetNX wrote over an existing key")
	}

	got, err := store.Get(ctx, "otp:cooldown")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "first" {
		t.Fatalf("Get = %q, want first", got)
	}
	if ttl := ttlOf(t, "otp:cooldown"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s, want the original TTL of 1m", ttl)
	}
}

func TestRedisStore_SetNXHasOneWinner(t *testing.T) {
	store := newStore(t)

	const callers = 20
	wins := make([]bool, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			wins[i], errs[i] = store.SetNX(context.Background(), "lock", []byte("v"), time.Minute)
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("SetNX: %v", err)
	}
	won := 0
	for _, w := range wins {
		if w {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d callers won, want 1", won)
	}
}

func TestRedisStore_GetDelConsumesKey(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "otp:code", []byte("hash"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.GetDel(ctx, "otp:code")
	if err != nil {
		t.Fatalf("GetDel: %v", err)
	}
	if string(got) != "hash" {
		t.Fatalf("GetDel = %q, want hash", got)
	}
	if _, err := store.GetDel(ctx, "otp:code"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second GetDel error = %v, want ErrNotFound", err)
	}
}

// A one-time code submitted twice at once must only be accepted once.
func TestRedisStore_GetDelHasOneWinner(t *testing.T) {
	store := newStore(t)

	if err := store.Set(context.Background(), "otp:code", []byte("hash"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	const callers = 20
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = store.GetDel(context.Background(), "otp:code")
		})
	}
	wg.Wait()

	consumed := 0
	for _, err := range errs {
		switch {
		case err == nil:
			consumed++
		case !errors.Is(err, domain.ErrNotFound):
			t.Fatalf("GetDel: %v", err)
		}
	}
	if consumed != 1 {
		t.Fatalf("code consumed %d times, want 1", consumed)
	}
}

func TestRedisStore_DeleteIsIdempotent(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "cache:k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, "cache:k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "cache:k"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, "cache:k"); err != nil {
		t.Fatalf("Delete of an absent key: %v", err)
	}
}

func TestRedisStore_IncrCountsWithinFixedWindow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := store.Incr(ctx, "ratelimit:k", time.Minute)
		if err != nil {
			t.Fatalf("Incr: %v", err)
		}
		if got != want {
			t.Fatalf("Incr = %d, want %d", got, want)
		}
	}

	// A later hit with a longer TTL must not push the window forward.
	if _, err := store.Incr(ctx, "ratelimit:k", time.Hour); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if ttl := ttlOf(t, "ratelimit:k"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s, want the first TTL of 1m", ttl)
	}
}

func TestRedisStore_IncrResetsAfterWindow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for range 2 {
		if _, err := store.Incr(ctx, "ratelimit:k", 50*time.Millisecond); err != nil {
			t.Fatalf("Incr: %v", err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	got, err := store.Incr(ctx, "ratelimit:k", time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if got != 1 {
		t.Fatalf("Incr after the window = %d, want 1", got)
	}
}

// Concurrent hits racing to create the counter must each get a distinct count,
// and the counter must still expire.
func TestRedisStore_IncrIsAtomic(t *testing.T) {
	store := newStore(t)

	const callers = 50
	counts := make([]int64, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			counts[i], errs[i] = store.Incr(context.Background(), "ratelimit:k", time.Minute)
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	slices.Sort(counts)
	for i, got := range counts {
		if got != int64(i+1) {
			t.Fatalf("sorted counts = %v, want 1 to %d each once", counts, callers)
		}
	}
	if ttl := ttlOf(t, "ratelimit:k"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s, want (0, 1m]", ttl)
	}
}

// A failed INCR inside the transaction must surface, not read as a count of 0.
func TestRedisStore_IncrOnNonCounterFails(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "cache:k", []byte("not a number"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := store.Incr(ctx, "cache:k", time.Minute); err == nil {
		t.Fatal("Incr succeeded on a non-numeric value, want an error")
	}
}

func TestRedisStore_CanceledContext(t *testing.T) {
	store := newStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Get(ctx, "cache:k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get error = %v, want context.Canceled", err)
	}
	if err := store.Set(ctx, "cache:k", []byte("v"), time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Set error = %v, want context.Canceled", err)
	}
	if got := ttlOf(t, "cache:k"); got != -2 {
		t.Fatalf("key exists with TTL %s, want it absent", got)
	}
}

// CLIENT PAUSE makes the real server stall, which is what a request deadline
// has to protect against. The pause is shorter than the client's read timeout,
// so only the deadline can end the call early, and it runs out by itself.
func TestRedisStore_HonorsContextDeadline(t *testing.T) {
	store := newStore(t)

	const pause = 500 * time.Millisecond
	if err := testRedis.Do(context.Background(), "CLIENT", "PAUSE", pause.Milliseconds(), "ALL").Err(); err != nil {
		t.Fatalf("CLIENT PAUSE: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := store.Get(ctx, "cache:k")
	elapsed := time.Since(start)

	if err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get error = %v, want a timeout", err)
	}
	if elapsed >= pause/2 {
		t.Fatalf("Get returned after %s, want it bounded by the 50ms deadline", elapsed)
	}
}

func TestNewRedisClient_FailsWhenServerUnreachable(t *testing.T) {
	cfg := *testCfg
	cfg.Redis.Port = 1 // nothing listens here

	if _, _, err := kvstore.NewRedisClient(&cfg); err == nil {
		t.Fatal("NewRedisClient succeeded, want an error")
	}
}

func TestNewRedisClient_RejectsNonPositiveTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		cfg := *testCfg
		cfg.Redis.Timeout = timeout

		if _, _, err := kvstore.NewRedisClient(&cfg); err == nil {
			t.Fatalf("NewRedisClient accepted timeout %s, want an error", timeout)
		}
	}
}

// redis-test speaks plaintext, so a client that demands TLS must refuse it
// rather than fall back.
func TestNewRedisClient_TLSDoesNotFallBackToPlaintext(t *testing.T) {
	cfg := *testCfg
	cfg.Redis.TLS = true

	if _, _, err := kvstore.NewRedisClient(&cfg); err == nil {
		t.Fatal("NewRedisClient connected over plaintext, want an error")
	}
}

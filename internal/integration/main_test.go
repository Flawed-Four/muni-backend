// Package integration runs the application against a real PostgreSQL, Mailpit
// and Redis (the postgres-test, mailpit-test and redis-test services in
// docker-compose.yml). Nothing is mocked: the HTTP stack is built through the
// same Wire graph used in production.
package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/di"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/kvstore"
)

var (
	testCfg     *config.Config
	testPool    *pgxpool.Pool
	testRedis   *redis.Client
	testHandler http.Handler
	mailpitURL  string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	gin.SetMode(gin.TestMode)
	testCfg = loadTestConfig()
	mailpitURL = fmt.Sprintf("http://%s:%d", testCfg.SMTP.Host, getenvInt("TEST_MAILPIT_PORT", 58025))

	if err := migrateUp(testCfg); err != nil {
		fmt.Fprintf(os.Stderr, "integration: cannot prepare test database: %v\n"+
			"Is it running? Start it with `make test-db-up`, or run everything with `make test`.\n", err)
		return 1
	}

	pool, closePool, err := database.NewPgxPool(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n", err)
		return 1
	}
	defer closePool()
	testPool = pool

	redisClient, closeRedis, err := kvstore.NewRedisClient(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n"+
			"Is it running? Start it with `make test-db-up`, or run everything with `make test`.\n", err)
		return 1
	}
	defer closeRedis()
	testRedis = redisClient

	app, cleanup, err := di.InitializeApp(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: build app: %v\n", err)
		return 1
	}
	defer cleanup()
	testHandler = app.Server.Handler

	return m.Run()
}

// loadTestConfig defaults match the postgres-test, mailpit-test and redis-test services in docker-compose.yml.
func loadTestConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{Env: "test", Port: 0},
		DB: config.DBConfig{
			Host:     getenv("TEST_DB_HOST", "localhost"),
			Port:     getenvInt("TEST_DB_PORT", 55432),
			User:     "muni_test",
			Password: "muni_test",
			Name:     "muni_test",
			SSLMode:  "disable",
		},
		Session: config.SessionConfig{TTL: time.Hour},
		SMTP: config.SMTPConfig{
			Host:        getenv("TEST_SMTP_HOST", "localhost"),
			Port:        getenvInt("TEST_SMTP_PORT", 51025),
			FromAddress: "noreply@muni.test",
			FromName:    "Muni Test",
			TLSMode:     "none",
			Timeout:     5 * time.Second,
		},
		Redis: config.RedisConfig{
			Host:    getenv("TEST_REDIS_HOST", "localhost"),
			Port:    getenvInt("TEST_REDIS_PORT", 56379),
			Timeout: time.Second,
		},
	}
}

// migrateUp applies the real migration files from db/migrations.
func migrateUp(cfg *config.Config) error {
	dir, err := filepath.Abs("../../db/migrations")
	if err != nil {
		return err
	}
	dsn := strings.Replace(cfg.DB.DSN(), "postgres://", "pgx5://", 1)

	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return fallback
}

// resetDB empties all tables so every test starts from a clean state.
// user_roles is deliberately not truncated: it holds rows seeded by the migrations.
func resetDB(t *testing.T) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), "TRUNCATE users, sessions, notes, note_tags CASCADE"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

// resetRedis empties the test Redis so every test starts from a clean state.
func resetRedis(t *testing.T) {
	t.Helper()
	if err := testRedis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
}

// Package integration runs the application against a real PostgreSQL instance
// (the postgres-test service in docker-compose.yml). Nothing is mocked: the
// HTTP stack is built through the same Wire graph used in production.
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

	"github.com/Flawed-Four/moni-be/internal/config"
	"github.com/Flawed-Four/moni-be/internal/di"
	"github.com/Flawed-Four/moni-be/internal/infrastructure/database"
)

var (
	testCfg     *config.Config
	testPool    *pgxpool.Pool
	testHandler http.Handler
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	gin.SetMode(gin.TestMode)
	testCfg = loadTestConfig()

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

	app, cleanup, err := di.InitializeApp(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: build app: %v\n", err)
		return 1
	}
	defer cleanup()
	testHandler = app.Server.Handler

	return m.Run()
}

// loadTestConfig defaults match the postgres-test service in docker-compose.yml.
func loadTestConfig() *config.Config {
	port, err := strconv.Atoi(getenv("TEST_DB_PORT", "55432"))
	if err != nil {
		port = 55432
	}
	return &config.Config{
		App: config.AppConfig{Env: "test", Port: 0},
		DB: config.DBConfig{
			Host:     getenv("TEST_DB_HOST", "localhost"),
			Port:     port,
			User:     "moni_test",
			Password: "moni_test",
			Name:     "moni_test",
			SSLMode:  "disable",
		},
		JWT: config.JWTConfig{Secret: "integration-test-secret", TTL: time.Hour},
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

// resetDB empties all tables so every test starts from a clean state.
func resetDB(t *testing.T) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), "TRUNCATE users"); err != nil {
		t.Fatalf("truncate users: %v", err)
	}
}

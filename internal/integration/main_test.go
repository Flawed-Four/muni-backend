// Package integration runs the application against a real PostgreSQL, Mailpit,
// Redis and Chroma (the postgres-test, mailpit-test, redis-test and chroma-test
// services in docker-compose.yml). Nothing is mocked: the HTTP stack is built
// through the same Wire graph used in production.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/vectorstore"
	"github.com/Flawed-Four/muni-backend/internal/logging"
)

var (
	testCfg     *config.Config
	testPool    *pgxpool.Pool
	testRedis   *redis.Client
	testHandler http.Handler
	mailpitURL  string
	// chromaDBURL lets tests call Chroma directly, bypassing the store.
	chromaDBURL string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	gin.SetMode(gin.TestMode)
	testCfg = loadTestConfig()

	// A real default logger (not slog's built-in one) lets captureLogs restore it cleanly.
	logger, err := logging.New(testCfg, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n", err)
		return 1
	}
	slog.SetDefault(logger)

	mailpitURL = fmt.Sprintf("http://%s:%d", testCfg.SMTP.Host, getenvInt("TEST_MAILPIT_PORT", 58025))
	chromaDBURL = fmt.Sprintf("http://%s:%d/api/v2/tenants/%s/databases/%s", testCfg.Chroma.Host, testCfg.Chroma.Port,
		url.PathEscape(testCfg.Chroma.Tenant), url.PathEscape(testCfg.Chroma.Database))

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

	_, closeChroma, err := vectorstore.NewChromaStore(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n"+
			"Is it running? Start it with `make test-db-up`, or run everything with `make test`.\n", err)
		return 1
	}
	closeChroma()

	app, cleanup, err := di.InitializeApp(testCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: build app: %v\n", err)
		return 1
	}
	defer cleanup()
	testHandler = app.Server.Handler

	return m.Run()
}

// loadTestConfig defaults match the postgres-test, mailpit-test, redis-test and chroma-test services in docker-compose.yml.
func loadTestConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{Env: "test", Port: 0},
		Log: config.LogConfig{Level: getenv("TEST_LOG_LEVEL", "info"), Format: "text"},
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
		Chroma: config.ChromaConfig{
			Host:     getenv("TEST_CHROMA_HOST", "localhost"),
			Port:     getenvInt("TEST_CHROMA_PORT", 58000),
			Tenant:   "default_tenant",
			Database: "default_database",
			Timeout:  5 * time.Second,
		},
		// Only TEST_GEMINI_API_KEY is read, so a key in .env is never billed by tests.
		Gemini: config.GeminiConfig{
			APIKey:        os.Getenv("TEST_GEMINI_API_KEY"),
			Model:         getenv("TEST_GEMINI_MODEL", "gemini-3.5-flash-lite"),
			ThinkingLevel: "low",
			Timeout:       60 * time.Second,
			MaxRetries:    2,
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

// stalledServer accepts connections but never answers, standing in for a hung server.
func stalledServer(t *testing.T) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return ln.Addr().(*net.TCPAddr)
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

// resetChroma deletes every collection so each test starts clean.
func resetChroma(t *testing.T) {
	t.Helper()
	for _, name := range chromaCollections(t) {
		req, err := http.NewRequest(http.MethodDelete, chromaDBURL+"/collections/"+url.PathEscape(name), nil)
		if err != nil {
			t.Fatalf("delete chroma collection %s: %v", name, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("delete chroma collection %s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delete chroma collection %s: status %d", name, resp.StatusCode)
		}
	}
}

func chromaCollections(t *testing.T) []string {
	t.Helper()
	resp, err := http.Get(chromaDBURL + "/collections")
	if err != nil {
		t.Fatalf("list chroma collections: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list chroma collections: status %d", resp.StatusCode)
	}

	var collections []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&collections); err != nil {
		t.Fatalf("decode chroma collections: %v", err)
	}
	names := make([]string, len(collections))
	for i, c := range collections {
		names[i] = c.Name
	}
	return names
}

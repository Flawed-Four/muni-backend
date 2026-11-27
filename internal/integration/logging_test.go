package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/middleware"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/response"
	"github.com/Flawed-Four/muni-backend/internal/logging"
)

// captureLogs sends slog's default logger to a JSON buffer until the test ends.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger, err := logging.New(&config.Config{Log: config.LogConfig{Level: "debug", Format: "json"}}, &buf)
	if err != nil {
		t.Fatalf("logging.New: %v", err)
	}
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}

// findLog returns the only log line with the given msg, failing otherwise.
func findLog(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, line := range logLines(t, buf) {
		if line["msg"] == msg {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d %q log lines, want 1; log:\n%s", len(found), msg, buf.String())
	}
	return found[0]
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequestLogLine(t *testing.T) {
	resetDB(t)
	user := register(t, "Harun", "harun@example.com", "s3cretpass")
	token := login(t, "harun@example.com", "s3cretpass")

	logs := captureLogs(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(middleware.RequestIDHeader, "trace-123")
	rec := serve(testHandler, req)
	wantStatus(t, rec, http.StatusOK)

	if got := rec.Header().Get(middleware.RequestIDHeader); got != "trace-123" {
		t.Fatalf("response request ID = %q, want the incoming trace-123", got)
	}

	line := findLog(t, logs, "request")
	want := map[string]any{
		"level":      "INFO",
		"method":     "GET",
		"route":      "/api/v1/users/me",
		"status":     float64(http.StatusOK),
		"ip":         "192.0.2.1", // httptest's default RemoteAddr
		"request_id": "trace-123",
		"user_id":    user.ID,
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("log %s = %v, want %v; line: %v", k, line[k], v, line)
		}
	}
	if _, ok := line["latency_ms"].(float64); !ok {
		t.Errorf("log latency_ms missing; line: %v", line)
	}
	if _, ok := line["err"]; ok {
		t.Errorf("successful request logged an err; line: %v", line)
	}
}

func TestLogsNeverContainSecrets(t *testing.T) {
	resetDB(t)
	logs := captureLogs(t)

	register(t, "Harun", "harun@example.com", "s3cretpass")
	token := login(t, "harun@example.com", "s3cretpass")
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me?token="+token, nil, token), http.StatusOK)
	wantStatus(t, do(t, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "harun@example.com", "password": "wrongpass1"}, ""), http.StatusUnauthorized)

	out := logs.String()
	if len(logLines(t, logs)) != 4 {
		t.Fatalf("want 4 request log lines; log:\n%s", out)
	}
	for _, secret := range []string{"s3cretpass", "wrongpass1", token} {
		if strings.Contains(out, secret) {
			t.Fatalf("log contains secret %q:\n%s", secret, out)
		}
	}
}

func TestRequestIDGeneratedWhenMissingOrInvalid(t *testing.T) {
	cases := map[string]string{
		"missing":     "",
		"bad chars":   "abc\ninjected=1",
		"too long":    strings.Repeat("a", 65),
		"with spaces": "a b",
	}
	seen := map[string]bool{}
	for name, incoming := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
			if incoming != "" {
				req.Header.Set(middleware.RequestIDHeader, incoming)
			}
			rec := serve(testHandler, req)
			wantStatus(t, rec, http.StatusUnauthorized)

			id := rec.Header().Get(middleware.RequestIDHeader)
			if id == "" || id == incoming || seen[id] {
				t.Fatalf("request ID = %q, want a fresh generated ID", id)
			}
			seen[id] = true

			line := findLog(t, logs, "request")
			if line["request_id"] != id || line["level"] != "INFO" || line["status"] != float64(http.StatusUnauthorized) {
				t.Fatalf("unexpected log line for request ID %q: %v", id, line)
			}
			if _, ok := line["user_id"]; ok {
				t.Fatalf("unauthenticated request logged a user_id: %v", line)
			}
		})
	}
}

func TestHealthCheckNotLogged(t *testing.T) {
	logs := captureLogs(t)
	wantStatus(t, do(t, http.MethodGet, "/api/v1/health", nil, ""), http.StatusOK)
	if logs.Len() != 0 {
		t.Fatalf("successful health check was logged:\n%s", logs.String())
	}
}

// The app has no route that fails on purpose, so this mounts the real
// middleware on a small engine with routes that error and panic.
func TestInternalErrorAndPanicLogged(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestLogger(), middleware.Recovery())
	r.GET("/fail", func(c *gin.Context) { response.Fail(c, errors.New("disk on fire")) })
	r.GET("/panic", func(c *gin.Context) { panic("boom") })

	t.Run("error", func(t *testing.T) {
		logs := captureLogs(t)
		rec := serve(r, httptest.NewRequest(http.MethodGet, "/fail", nil))
		wantStatus(t, rec, http.StatusInternalServerError)
		if msg := errorMessage(t, rec); msg != "internal server error" {
			t.Fatalf("error message = %q, internals must not leak", msg)
		}

		line := findLog(t, logs, "request")
		if line["level"] != "ERROR" || line["route"] != "/fail" || line["err"] != "disk on fire" ||
			line["request_id"] != rec.Header().Get(middleware.RequestIDHeader) {
			t.Fatalf("unexpected log line: %v", line)
		}
	})

	t.Run("panic", func(t *testing.T) {
		logs := captureLogs(t)
		rec := serve(r, httptest.NewRequest(http.MethodGet, "/panic", nil))
		wantStatus(t, rec, http.StatusInternalServerError)
		if msg := errorMessage(t, rec); msg != "internal server error" {
			t.Fatalf("error message = %q, want the generic one", msg)
		}

		id := rec.Header().Get(middleware.RequestIDHeader)
		p := findLog(t, logs, "panic recovered")
		if p["level"] != "ERROR" || p["panic"] != "boom" || p["request_id"] != id ||
			!strings.Contains(p["stack"].(string), "logging_test.go") {
			t.Fatalf("unexpected panic line: %v", p)
		}
		line := findLog(t, logs, "request")
		if line["level"] != "ERROR" || line["err"] != "panic: boom" || line["request_id"] != id {
			t.Fatalf("unexpected request line: %v", line)
		}
	})
}

func TestLoggingConfig(t *testing.T) {
	newLogger := func(env, level, format string) (*slog.Logger, *bytes.Buffer, error) {
		var buf bytes.Buffer
		l, err := logging.New(&config.Config{
			App: config.AppConfig{Env: env},
			Log: config.LogConfig{Level: level, Format: format},
		}, &buf)
		return l, &buf, err
	}

	t.Run("invalid values are rejected", func(t *testing.T) {
		if _, _, err := newLogger("production", "verbose", ""); err == nil {
			t.Fatal("LOG_LEVEL=verbose accepted")
		}
		if _, _, err := newLogger("production", "info", "xml"); err == nil {
			t.Fatal("LOG_FORMAT=xml accepted")
		}
	})

	t.Run("format follows the environment", func(t *testing.T) {
		l, buf, err := newLogger("production", "info", "")
		if err != nil {
			t.Fatal(err)
		}
		l.Info("hello")
		if !json.Valid(buf.Bytes()) {
			t.Fatalf("production log is not JSON: %s", buf.String())
		}

		l, buf, err = newLogger("development", "info", "")
		if err != nil {
			t.Fatal(err)
		}
		l.Info("hello")
		if !strings.Contains(buf.String(), "level=INFO msg=hello") {
			t.Fatalf("development log is not text: %s", buf.String())
		}
	})

	t.Run("level filters records", func(t *testing.T) {
		l, buf, err := newLogger("production", "WARN", "json")
		if err != nil {
			t.Fatal(err)
		}
		l.Info("hidden")
		l.Warn("shown")
		if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
			t.Fatalf("level warn not applied: %s", buf.String())
		}
	})

	t.Run("context fields survive logger.With", func(t *testing.T) {
		l, buf, err := newLogger("production", "info", "json")
		if err != nil {
			t.Fatal(err)
		}
		ctx := logging.With(context.Background(), slog.String("request_id", "abc"))
		l.With("component", "notes").InfoContext(ctx, "hello")

		var line map[string]any
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		if line["request_id"] != "abc" || line["component"] != "notes" {
			t.Fatalf("context fields lost: %v", line)
		}
	})
}

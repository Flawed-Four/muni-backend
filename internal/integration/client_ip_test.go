package integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/di"
)

// peerIP is the RemoteAddr host that httptest.NewRequest uses.
const peerIP = "192.0.2.1"

// newTestHandler builds the app through the production Wire graph with a changed copy of testCfg.
func newTestHandler(t *testing.T, change func(*config.Config)) http.Handler {
	t.Helper()
	cfg := *testCfg
	change(&cfg)
	app, cleanup, err := di.InitializeApp(&cfg)
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	t.Cleanup(cleanup)
	return app.Server.Handler
}

// loginIP logs in through h with the given headers and returns the IP stored for the session.
// It also checks that the request log line has the same ip.
func loginIP(t *testing.T, h http.Handler, headers map[string]string) string {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), "DELETE FROM sessions"); err != nil {
		t.Fatalf("delete sessions: %v", err)
	}
	logs := captureLogs(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewBufferString(`{"email":"harun@example.com","password":"s3cretpass"}`))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := serve(h, req)
	wantStatus(t, rec, http.StatusOK)

	var ip string
	err := testPool.QueryRow(context.Background(),
		"SELECT COALESCE(host(ip_address), '') FROM sessions").Scan(&ip)
	if err != nil {
		t.Fatalf("query session ip: %v", err)
	}
	if logged := findLog(t, logs, "request")["ip"]; logged != ip {
		t.Fatalf("logged ip = %v, stored ip = %q", logged, ip)
	}
	return ip
}

func TestLoginClientIP(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")

	// Headers a client can send to pretend to be someone else.
	spoofed := map[string]string{
		"X-Forwarded-For":  "198.51.100.9",
		"X-Real-IP":        "198.51.100.10",
		"CF-Connecting-IP": "198.51.100.11",
	}

	t.Run("no trusted proxy ignores forwarding headers", func(t *testing.T) {
		if got := loginIP(t, testHandler, spoofed); got != peerIP {
			t.Fatalf("ip = %q, want peer %q", got, peerIP)
		}
	})

	t.Run("peer outside the trusted list is not believed", func(t *testing.T) {
		h := newTestHandler(t, func(c *config.Config) { c.App.TrustedProxies = []string{"10.0.0.0/8"} })
		if got := loginIP(t, h, spoofed); got != peerIP {
			t.Fatalf("ip = %q, want peer %q", got, peerIP)
		}
	})

	t.Run("trusted proxy forwards the client address", func(t *testing.T) {
		// Spaces and empty entries come from env values like "10.0.0.0/8, 192.0.2.1,".
		h := newTestHandler(t, func(c *config.Config) {
			c.App.TrustedProxies = []string{"10.0.0.0/8", " " + peerIP + " ", ""}
		})
		// The proxy appends the address it saw, so the leftmost entry is the client's own claim
		// and the rightmost untrusted entry is the real client.
		got := loginIP(t, h, map[string]string{"X-Forwarded-For": "203.0.113.66, 198.51.100.9"})
		if got != "198.51.100.9" {
			t.Fatalf("ip = %q, want forwarded %q", got, "198.51.100.9")
		}
	})

	t.Run("trusted platform header is used", func(t *testing.T) {
		h := newTestHandler(t, func(c *config.Config) { c.App.TrustedPlatform = "CF-Connecting-IP" })
		if got := loginIP(t, h, spoofed); got != "198.51.100.11" {
			t.Fatalf("ip = %q, want %q", got, "198.51.100.11")
		}
	})
}

func TestInvalidTrustedProxyStopsBoot(t *testing.T) {
	cfg := *testCfg
	cfg.App.TrustedProxies = []string{peerIP, "not-an-ip"}

	_, cleanup, err := di.InitializeApp(&cfg)
	if err == nil {
		cleanup()
		t.Fatal("InitializeApp accepted an invalid TRUSTED_PROXIES entry")
	}
	if !strings.Contains(err.Error(), "TRUSTED_PROXIES") || !strings.Contains(err.Error(), "not-an-ip") {
		t.Fatalf("error = %q, want it to name TRUSTED_PROXIES and the bad entry", err)
	}
}

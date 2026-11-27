package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Flawed-Four/muni-backend/internal/delivery/http/dto"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/response"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/security"
)

func do(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	return rec
}

func data[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return env.Data
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e response.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return e.Error.Message
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func register(t *testing.T, name, email, password string) dto.UserResponse {
	t.Helper()
	rec := do(t, http.MethodPost, "/api/v1/auth/register",
		dto.RegisterRequest{Name: name, Email: email, Password: password}, "")
	wantStatus(t, rec, http.StatusCreated)
	return data[dto.UserResponse](t, rec)
}

func login(t *testing.T, email, password string) string {
	t.Helper()
	rec := do(t, http.MethodPost, "/api/v1/auth/login",
		dto.LoginRequest{Email: email, Password: password}, "")
	wantStatus(t, rec, http.StatusOK)
	tok := data[dto.TokenResponse](t, rec)
	if tok.AccessToken == "" || tok.TokenType != "Bearer" {
		t.Fatalf("unexpected token response: %+v", tok)
	}
	return tok.AccessToken
}

func TestHealth(t *testing.T) {
	rec := do(t, http.MethodGet, "/health", nil, "")
	wantStatus(t, rec, http.StatusOK)
}

func TestRegisterLoginMe(t *testing.T) {
	resetDB(t)

	user := register(t, "Harun", "Harun@Example.com", "s3cretpass")
	if user.ID == "" || user.Email != "harun@example.com" || user.Role != "user" {
		t.Fatalf("unexpected user: %+v", user)
	}

	token := login(t, "HARUN@example.com", "s3cretpass")

	rec := do(t, http.MethodGet, "/api/v1/users/me", nil, token)
	wantStatus(t, rec, http.StatusOK)
	me := data[dto.UserResponse](t, rec)
	if me.ID != user.ID || me.Name != "Harun" || me.Role != "user" {
		t.Fatalf("me = %+v, want id %s", me, user.ID)
	}
}

func TestRegisterStoresHashedPassword(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")

	var hash string
	err := testPool.QueryRow(context.Background(),
		"SELECT password_hash FROM users WHERE email = $1", "harun@example.com").Scan(&hash)
	if err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if hash == "s3cretpass" {
		t.Fatal("password stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cretpass")); err != nil {
		t.Fatalf("stored hash does not match password: %v", err)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")

	rec := do(t, http.MethodPost, "/api/v1/auth/register",
		dto.RegisterRequest{Name: "Other", Email: "HARUN@example.com", Password: "anotherpass"}, "")
	wantStatus(t, rec, http.StatusConflict)
}

func TestRegisterValidation(t *testing.T) {
	resetDB(t)

	cases := map[string]dto.RegisterRequest{
		"missing name":   {Email: "a@example.com", Password: "s3cretpass"},
		"invalid email":  {Name: "A", Email: "not-an-email", Password: "s3cretpass"},
		"short password": {Name: "A", Email: "a@example.com", Password: "short"},
		"long password":  {Name: "A", Email: "a@example.com", Password: string(make([]byte, 73))},
		// 40 characters but 80 bytes: within max=72 characters, over bcrypt's 72 bytes.
		"multibyte password over 72 bytes": {Name: "A", Email: "a@example.com", Password: strings.Repeat("é", 40)},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, do(t, http.MethodPost, "/api/v1/auth/register", body, ""), http.StatusBadRequest)
		})
	}

	t.Run("malformed json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		testHandler.ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusBadRequest)
	})
}

func TestLoginFailures(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")

	wrongPass := do(t, http.MethodPost, "/api/v1/auth/login",
		dto.LoginRequest{Email: "harun@example.com", Password: "wrongpass"}, "")
	unknown := do(t, http.MethodPost, "/api/v1/auth/login",
		dto.LoginRequest{Email: "nobody@example.com", Password: "s3cretpass"}, "")

	wantStatus(t, wrongPass, http.StatusUnauthorized)
	wantStatus(t, unknown, http.StatusUnauthorized)
	if errorMessage(t, wrongPass) != errorMessage(t, unknown) {
		t.Fatal("wrong password and unknown email must be indistinguishable")
	}
}

func TestMeRequiresValidToken(t *testing.T) {
	resetDB(t)

	t.Run("no header", func(t *testing.T) {
		wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, ""), http.StatusUnauthorized)
	})
	t.Run("garbage token", func(t *testing.T) {
		wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, "abc"), http.StatusUnauthorized)
	})
	t.Run("well-formed token that was never issued", func(t *testing.T) {
		tok, _, err := security.NewSessionTokens().New()
		if err != nil {
			t.Fatal(err)
		}
		wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, tok), http.StatusUnauthorized)
	})
}

func TestLoginStoresOnlyTokenHash(t *testing.T) {
	resetDB(t)
	user := register(t, "Harun", "harun@example.com", "s3cretpass")
	token := login(t, "harun@example.com", "s3cretpass")

	var userID string
	var hash []byte
	err := testPool.QueryRow(context.Background(),
		"SELECT user_id, token_hash FROM sessions").Scan(&userID, &hash)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if userID != user.ID {
		t.Fatalf("session user_id = %s, want %s", userID, user.ID)
	}
	if string(hash) == token || !bytes.Equal(hash, security.NewSessionTokens().Hash(token)) {
		t.Fatal("sessions.token_hash must be the SHA-256 of the token, not the token")
	}
}

func TestLogoutRevokesOnlyCurrentSession(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")
	first := login(t, "harun@example.com", "s3cretpass")
	second := login(t, "harun@example.com", "s3cretpass")

	wantStatus(t, do(t, http.MethodPost, "/api/v1/auth/logout", nil, first), http.StatusNoContent)

	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, first), http.StatusUnauthorized)
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, second), http.StatusOK)

	// A revoked token cannot log out again.
	wantStatus(t, do(t, http.MethodPost, "/api/v1/auth/logout", nil, first), http.StatusUnauthorized)
}

func TestLogoutAllRevokesEveryUserSession(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")
	register(t, "Other", "other@example.com", "s3cretpass")
	a := login(t, "harun@example.com", "s3cretpass")
	b := login(t, "harun@example.com", "s3cretpass")
	other := login(t, "other@example.com", "s3cretpass")

	wantStatus(t, do(t, http.MethodPost, "/api/v1/auth/logout-all", nil, a), http.StatusNoContent)

	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, a), http.StatusUnauthorized)
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, b), http.StatusUnauthorized)
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, other), http.StatusOK)
}

func TestExpiredSessionIsRejected(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")
	token := login(t, "harun@example.com", "s3cretpass")

	if _, err := testPool.Exec(context.Background(),
		"UPDATE sessions SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, token), http.StatusUnauthorized)
}

func TestSessionRecordsClientAndExpiry(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewBufferString(`{"email":"harun@example.com","password":"s3cretpass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "muni-test/1.0")
	req.RemoteAddr = "203.0.113.7:4321"
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	wantStatus(t, rec, http.StatusOK)

	tok := data[dto.TokenResponse](t, rec)
	if d := time.Until(tok.ExpiresAt); d <= 0 || d > testCfg.Session.TTL {
		t.Fatalf("expires_at = %v, want within (0, %v] from now", tok.ExpiresAt, testCfg.Session.TTL)
	}

	var ua, ip string
	err := testPool.QueryRow(context.Background(),
		"SELECT user_agent, host(ip_address) FROM sessions").Scan(&ua, &ip)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if ua != "muni-test/1.0" || ip != "203.0.113.7" {
		t.Fatalf("user_agent=%q ip=%q", ua, ip)
	}
}

func TestDeletingUserCascadesSessions(t *testing.T) {
	resetDB(t)
	register(t, "Harun", "harun@example.com", "s3cretpass")
	token := login(t, "harun@example.com", "s3cretpass")

	if _, err := testPool.Exec(context.Background(), "DELETE FROM users"); err != nil {
		t.Fatalf("delete users: %v", err)
	}
	wantStatus(t, do(t, http.MethodGet, "/api/v1/users/me", nil, token), http.StatusUnauthorized)
}

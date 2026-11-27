package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database"
)

func TestNewPgxPool_PasswordWithURLCharacters(t *testing.T) {
	ctx := context.Background()
	const role = "muni_dsn_test"
	// Characters that mean something in a URL, plus "%41", which must not be decoded to "A".
	const password = "p@ss:w/rd?#%41&é"

	drop := func() {
		if _, err := testPool.Exec(context.Background(), "DROP ROLE IF EXISTS "+role); err != nil {
			t.Fatalf("drop role: %v", err)
		}
	}
	drop()
	t.Cleanup(drop)
	if _, err := testPool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", role, password)); err != nil {
		t.Fatalf("create role: %v", err)
	}

	cfg := *testCfg
	cfg.DB.User = role
	cfg.DB.Password = password
	pool, closePool, err := database.NewPgxPool(&cfg)
	if err != nil {
		t.Fatalf("connect with special password: %v", err)
	}
	defer closePool()

	var user string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&user); err != nil {
		t.Fatalf("query: %v", err)
	}
	if user != role {
		t.Fatalf("connected as %q, want %q", user, role)
	}
}

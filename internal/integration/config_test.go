package integration

import (
	"testing"

	"github.com/Flawed-Four/muni-backend/internal/config"
)

func TestLoadPort(t *testing.T) {
	t.Setenv("DB_USER", "muni")
	t.Setenv("DB_PASSWORD", "muni")
	t.Setenv("DB_NAME", "muni")
	t.Setenv("APP_PORT", "9000")

	for _, tc := range []struct {
		name, port string
		want       int
	}{
		{"empty PORT keeps APP_PORT", "", 9000},
		{"PORT replaces APP_PORT", "3000", 3000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORT", tc.port)
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.App.Port != tc.want {
				t.Fatalf("port = %d, want %d", cfg.App.Port, tc.want)
			}
		})
	}
}

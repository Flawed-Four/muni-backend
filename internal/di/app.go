package di

import (
	"net/http"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/usecase"
)

// App is the root of the dependency graph assembled by Wire.
type App struct {
	Server *http.Server
}

// ProvideSessionTTL exposes the configured session lifetime to the usecase layer.
func ProvideSessionTTL(cfg *config.Config) usecase.SessionTTL {
	return usecase.SessionTTL(cfg.Session.TTL)
}

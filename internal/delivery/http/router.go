package http

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/Flawed-Four/muni-backend/docs"
	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/handler"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/middleware"
	"github.com/Flawed-Four/muni-backend/internal/usecase"
)

func NewRouter(
	cfg *config.Config,
	authUC *usecase.AuthUsecase,
	health *handler.HealthHandler,
	auth *handler.AuthHandler,
	user *handler.UserHandler,
) (*gin.Engine, error) {
	// Must run before gin.New, which already prints debug output.
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Gin trusts every proxy by default, which lets any client pick its own
	// ClientIP through X-Forwarded-For. A nil list trusts none.
	if err := r.SetTrustedProxies(trustedProxies(cfg.App.TrustedProxies)); err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	r.TrustedPlatform = strings.TrimSpace(cfg.App.TrustedPlatform)
	r.Use(middleware.RequestLogger(), middleware.Recovery())

	r.GET("/health", health.Check)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", auth.Register)
		v1.POST("/auth/login", auth.Login)

		protected := v1.Group("", middleware.Auth(authUC))
		protected.POST("/auth/logout", auth.Logout)
		protected.POST("/auth/logout-all", auth.LogoutAll)
		protected.GET("/users/me", user.Me)
	}

	return r, nil
}

// trustedProxies trims the entries and drops empty ones, so "a, b," works.
// It returns nil when nothing is left.
func trustedProxies(entries []string) []string {
	var out []string
	for _, e := range entries {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

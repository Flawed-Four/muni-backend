package http

import (
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
) *gin.Engine {
	// Must run before gin.New, which already prints debug output.
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
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

	return r
}

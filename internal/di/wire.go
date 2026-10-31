//go:build wireinject

package di

import (
	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/moni-be/internal/config"
	deliveryhttp "github.com/Flawed-Four/moni-be/internal/delivery/http"
	"github.com/Flawed-Four/moni-be/internal/delivery/http/handler"
	"github.com/Flawed-Four/moni-be/internal/domain"
	"github.com/Flawed-Four/moni-be/internal/infrastructure/database"
	"github.com/Flawed-Four/moni-be/internal/infrastructure/security"
	"github.com/Flawed-Four/moni-be/internal/repository/postgres"
	"github.com/Flawed-Four/moni-be/internal/usecase"
)

var infraSet = wire.NewSet(
	database.NewPgxPool,
	wire.Bind(new(handler.Pinger), new(*pgxpool.Pool)),
	security.NewJWTService,
	wire.Bind(new(domain.TokenService), new(*security.JWTService)),
	security.NewBcryptHasher,
	wire.Bind(new(domain.PasswordHasher), new(*security.BcryptHasher)),
)

var repoSet = wire.NewSet(
	postgres.NewUserRepository,
	wire.Bind(new(domain.UserRepository), new(*postgres.UserRepository)),
)

var usecaseSet = wire.NewSet(
	usecase.NewAuthUsecase,
	usecase.NewUserUsecase,
)

var handlerSet = wire.NewSet(
	handler.NewHealthHandler,
	handler.NewAuthHandler,
	handler.NewUserHandler,
)

var httpSet = wire.NewSet(
	deliveryhttp.NewRouter,
	deliveryhttp.NewServer,
)

func InitializeApp(cfg *config.Config) (*App, func(), error) {
	wire.Build(
		infraSet,
		repoSet,
		usecaseSet,
		handlerSet,
		httpSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil, nil
}

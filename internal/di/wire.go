//go:build wireinject

package di

import (
	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/muni-backend/internal/config"
	deliveryhttp "github.com/Flawed-Four/muni-backend/internal/delivery/http"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/handler"
	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/email"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/kvstore"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/llm"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/security"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/vectorstore"
	"github.com/Flawed-Four/muni-backend/internal/repository/postgres"
	"github.com/Flawed-Four/muni-backend/internal/usecase"
)

var infraSet = wire.NewSet(
	database.NewPgxPool,
	wire.Bind(new(handler.Pinger), new(*pgxpool.Pool)),
	security.NewSessionTokens,
	wire.Bind(new(domain.TokenService), new(*security.SessionTokens)),
	security.NewBcryptHasher,
	wire.Bind(new(domain.PasswordHasher), new(*security.BcryptHasher)),
	email.NewSMTPSender,
	wire.Bind(new(domain.EmailSender), new(*email.SMTPSender)),
	kvstore.NewRedisClient,
	kvstore.NewRedisStore,
	wire.Bind(new(domain.KeyValueStore), new(*kvstore.RedisStore)),
	vectorstore.NewChromaStore,
	wire.Bind(new(domain.VectorStore), new(*vectorstore.ChromaStore)),
	llm.NewGemini,
	wire.Bind(new(domain.LanguageModel), new(*llm.Gemini)),
)

var repoSet = wire.NewSet(
	postgres.NewUserRepository,
	wire.Bind(new(domain.UserRepository), new(*postgres.UserRepository)),
	postgres.NewSessionRepository,
	wire.Bind(new(domain.SessionRepository), new(*postgres.SessionRepository)),
	postgres.NewNoteRepository,
	wire.Bind(new(domain.NoteRepository), new(*postgres.NoteRepository)),
	postgres.NewNoteTagRepository,
	wire.Bind(new(domain.NoteTagRepository), new(*postgres.NoteTagRepository)),
)

var usecaseSet = wire.NewSet(
	ProvideSessionTTL,
	usecase.NewAuthUsecase,
	usecase.NewUserUsecase,
	usecase.NewNoteUsecase,
	usecase.NewNoteTagUsecase,
)

var handlerSet = wire.NewSet(
	handler.NewHealthHandler,
	handler.NewAuthHandler,
	handler.NewUserHandler,
	handler.NewNoteHandler,
	handler.NewNoteTagHandler,
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

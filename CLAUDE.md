# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run                      # go run ./cmd/api (reads .env; copy from .env.example first)
make docker-up                # dev Postgres + migrations + Mailpit via compose (docker-down to stop)
make test                     # starts postgres-test + mailpit-test, runs ALL tests, always tears them down
make test-db-up               # start only the test services (Postgres 55432, Mailpit SMTP 51025 / API 58025)
make test-run                 # go test ./... against already-running test services
go test -count=1 ./internal/integration -run TestRegisterLoginMe   # single test (test services must be up)
make lint                     # go vet ./...
make generate                 # sqlc + swag + wire (see "Generated code")
make migrate-create name=xxx  # new sequential migration in db/migrations
```

Codegen tools (sqlc, wire, swag, migrate) run via `go run <tool>@<pinned version>`; versions are pinned at the top of the `Makefile`. No global install needed.

## Architecture

Clean architecture; dependencies point inward:

```
delivery/http ──► usecase ──► domain ◄── repository/postgres
                                 ▲
                                 └── infrastructure (session tokens, bcrypt, pgx pool, SMTP)
```

- `internal/domain` holds entities, the interfaces (`UserRepository`, `SessionRepository`, `TokenService`, `PasswordHasher`, `EmailSender`) and sentinel errors. It must not import other internal packages. `usecase` imports only `domain`.
- Errors flow upward as domain sentinels (`ErrNotFound`, `ErrEmailTaken`, `ErrInvalidCredentials`, `ErrInvalidToken`). Repositories translate DB errors into them (unique violation → `ErrEmailTaken`; no rows and malformed UUID → `ErrNotFound`). Match Postgres error codes with `errors.As(err, &*pgconn.PgError)` and the constants from `github.com/jackc/pgerrcode` (e.g. `pgerrcode.UniqueViolation`) — never string literals like `"23505"` or new local constants. `delivery/http/response.Fail` is the single place mapping them to HTTP status; unknown errors become a logged 500.
- Handlers wrap payloads as `{"data": ...}` / `{"error": {"message": ...}}`.
- `internal/di/wire.go` (build tag `wireinject`) defines provider sets per layer; `wire_gen.go` is the generated result. `App` (in `app.go`) holds the `*http.Server`. A new interface binding needs a `wire.Bind` in the same set as the concrete provider (e.g. `handler.Pinger` ← `*pgxpool.Pool` is bound in `infraSet`), otherwise Wire fails.
- `cmd/api/main.go` loads config, calls `di.InitializeApp`, serves with graceful shutdown. It also carries the swag general-API annotations.
- Config is env-only (`internal/config`, `caarlos0/env`; `.env` is auto-loaded via godotenv).

## Adding a feature (touches every layer, in this order)

1. `make migrate-create name=...` and write the `up`/`down` SQL.
2. Add queries to `db/queries/*.sql` → `make sqlc`.
3. Domain: entity + repository interface (+ new sentinel errors) in `internal/domain`.
4. Repository in `internal/repository/postgres`: wrap the sqlc queries, map DB errors to domain sentinels, convert sqlc rows with a `toDomain`.
5. Usecase in `internal/usecase`, depending only on domain interfaces.
6. DTOs in `delivery/http/dto`, handler in `delivery/http/handler`, route in `router.go` (add new handlers to `NewRouter`'s params). New sentinel errors need a case in `response.Fail`.
7. Register constructors (and `wire.Bind` for interfaces) in `internal/di/wire.go` → `make wire`.
8. Swag annotations on the handler → `make swag`.
9. Integration tests in `internal/integration`; add new tables to the `TRUNCATE` in `resetDB`.

## Generated code — do not hand-edit

| Output | Source | Regenerate |
|---|---|---|
| `internal/infrastructure/database/sqlc/` | `db/queries/*.sql` + `db/migrations` (schema) | `make sqlc` |
| `internal/di/wire_gen.go` | `internal/di/wire.go` | `make wire` |
| `docs/` (Swagger) | `@` annotations on handlers and `cmd/api/main.go` | `make swag` |

`internal/delivery/http/router.go` imports `docs` for its side effect, so the project does not build until `docs/` exists. sqlc maps `uuid` to Go `string`; `down.sql` files are ignored by sqlc.

## Testing

All tests are integration tests in `internal/integration` against a real PostgreSQL and a real Mailpit (compose services `postgres-test` and `mailpit-test`, profile `test`) — **no mocks**; keep it that way. `TestMain` applies the real migrations with golang-migrate, then builds the HTTP handler via `di.InitializeApp`, i.e. the production Wire graph. Tests call `resetDB(t)` (TRUNCATE) first, and email tests call `resetMail(t)` and read the result through Mailpit's REST API. They run sequentially because they share one DB and one inbox, so don't add `t.Parallel()` or a second package that uses them. If either service is down the suite fails (it does not skip). Override with `TEST_DB_PORT` / `TEST_SMTP_PORT` / `TEST_MAILPIT_PORT` / `POSTGRES_IMAGE`.

Mailpit in the suite runs plaintext without auth, so the TLS and AUTH paths of `SMTPSender` are not covered there beyond checking that `starttls` refuses to fall back to plaintext.

## Gotchas

- Auth is stateful: `login` inserts a row in `sessions` and returns an opaque random token; only its SHA-256 is stored. The `Auth` middleware calls `AuthUsecase.Authenticate`, which hits the DB on every request (and bumps `last_used_at`). Unknown, revoked and expired sessions all surface as `ErrInvalidToken`.

- Login returns the same `ErrInvalidCredentials` for unknown email and wrong password (no user enumeration); tests assert this.

- `domain.EmailSender` is bound in `infraSet`, but nothing consumes it yet, so Wire leaves `NewSMTPSender` out of `InitializeApp` and its config validation does not run at startup. Once a usecase takes `domain.EmailSender`, an invalid `SMTP_FROM_ADDRESS` or `SMTP_TLS_MODE` stops the app from booting.

- `EmailSender.Send` is synchronous and opens a new connection per call, so calling it from a request path adds the SMTP round trip to the response time.

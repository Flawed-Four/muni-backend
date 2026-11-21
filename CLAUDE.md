# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run                      # go run ./cmd/api (reads .env; copy from .env.example first)
make docker-up                # dev Postgres + migrations + Mailpit + Redis + Chroma via compose (docker-down to stop)
make test                     # starts postgres-test + mailpit-test + redis-test + chroma-test, runs ALL tests, always tears them down
make test-db-up               # start only the test services (Postgres 55432, Mailpit SMTP 51025 / API 58025, Redis 56379, Chroma 58000)
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
                                 └── infrastructure (session tokens, bcrypt, pgx pool, SMTP, Redis, Chroma, Gemini)
```

- `internal/domain` holds entities, the interfaces (`UserRepository`, `SessionRepository`, `TokenService`, `PasswordHasher`, `EmailSender`, `KeyValueStore`, `VectorStore`, `LanguageModel`) and sentinel errors. It must not import other internal packages. `usecase` imports only `domain`.
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

All tests are integration tests in `internal/integration` against a real PostgreSQL, a real Mailpit, a real Redis, a real Chroma and the real Gemini API (compose services `postgres-test`, `mailpit-test`, `redis-test` and `chroma-test`, profile `test`) — **no mocks**; keep it that way. `TestMain` applies the real migrations with golang-migrate, then builds the HTTP handler via `di.InitializeApp`, i.e. the production Wire graph. Tests call `resetDB(t)` (TRUNCATE) first, email tests call `resetMail(t)` and read the result through Mailpit's REST API, Redis tests call `resetRedis(t)` (FLUSHDB), and Chroma tests call `resetChroma(t)` (deletes every collection in the test database). They run sequentially because they share one DB, one inbox, one Redis and one Chroma, so don't add `t.Parallel()` or a second package that uses them. If any service is down the suite fails (it does not skip). The one exception is Gemini: it is a paid API that cannot run in compose, so live Gemini tests skip unless `TEST_GEMINI_API_KEY` is set (`TEST_GEMINI_MODEL` overrides the model). Tests never read `GEMINI_API_KEY`, so a key in `.env` is not billed. Override with `TEST_DB_PORT` / `TEST_SMTP_PORT` / `TEST_MAILPIT_PORT` / `TEST_REDIS_PORT` / `TEST_CHROMA_PORT` / `POSTGRES_IMAGE` / `REDIS_IMAGE` / `CHROMA_IMAGE`.

Mailpit in the suite runs plaintext without auth, so the TLS and AUTH paths of `SMTPSender` are not covered there beyond checking that `starttls` refuses to fall back to plaintext. The same goes for `redis-test` and `NewRedisClient`, and for `chroma-test` and `NewChromaStore` (no TLS, no API key): only the refusal to fall back from TLS is covered. The Chroma timeout test uses a local TCP listener that never answers, because Chroma cannot be stalled on purpose; it checks the client, not Chroma. The Gemini timeout test does the same through `GEMINI_BASE_URL` (`stalledServer` in `main_test.go`).

## Gotchas

- Auth is stateful: `login` inserts a row in `sessions` and returns an opaque random token; only its SHA-256 is stored. The `Auth` middleware calls `AuthUsecase.Authenticate`, which hits the DB on every request (and bumps `last_used_at`). Unknown, revoked and expired sessions all surface as `ErrInvalidToken`.

- Login returns the same `ErrInvalidCredentials` for unknown email and wrong password (no user enumeration); tests assert this.

- `domain.EmailSender` is bound in `infraSet`, but nothing consumes it yet, so Wire leaves `NewSMTPSender` out of `InitializeApp` and its config validation does not run at startup. Once a usecase takes `domain.EmailSender`, an invalid `SMTP_FROM_ADDRESS` or `SMTP_TLS_MODE` stops the app from booting.

- `EmailSender.Send` is synchronous and opens a new connection per call, so calling it from a request path adds the SMTP round trip to the response time.

- `domain.KeyValueStore` is likewise bound in `infraSet` with no consumer yet, so Wire leaves `NewRedisClient` out of `InitializeApp` and the app neither connects to Redis nor needs it. Once a usecase takes `domain.KeyValueStore` (or anything takes `*redis.Client`), the app pings Redis at boot and refuses to start without it. `/health` does not check Redis.

- `KeyValueStore` rejects a non-positive TTL because go-redis reads `0` as "never expire" and `-1` as "keep the current TTL". `Incr` has to stay a single transaction for the same reason: a counter left without a TTL would lock its subject out for good.

- OTP codes and rate-limit counters share Redis with cache entries, so a production Redis with `maxmemory` must run `noeviction`. An evicting policy can drop them under memory pressure, which invalidates a pending code or resets a limit.

- `domain.VectorStore` is bound in `infraSet` with no consumer yet, so Wire leaves `NewChromaStore` out of `InitializeApp` and the app neither connects to Chroma nor needs it. Once a usecase takes `domain.VectorStore`, the app checks Chroma's heartbeat and the configured tenant/database at boot and refuses to start without them. `/health` does not check Chroma.

- `ChromaStore` is a thin `net/http` client for the Chroma REST API v2, not `chroma-go`: that library creates a default ONNX embedding function (downloading onnxruntime and a model at runtime) whenever a collection is created without one, which fails in the distroless/static image. Chroma computes no embeddings itself here, so callers pass vectors.

- A collection is created by its first `Upsert` with cosine distance, and that write fixes the embedding dimension; switching embedding models means a new collection. `Query`/`Delete`/`DeleteWhere` on a missing collection find nothing and do not create it. Collection IDs are cached per store; a 404 drops the cached ID and retries once, so a collection recreated outside the app is picked up.

- Chroma's upsert merges metadata: new keys overwrite, omitted old keys stay. The document is always sent (an empty string clears it). Floats are encoded with a decimal point (`jsonFloat`) because `encoding/json` writes `3.0` as `3`, which Chroma would store as an integer.

- Chroma 1.5.9 keeps about four files open per collection segment, deleted collections included until it restarts, so the default `nofile` of 1024 runs out after a few hundred collections and every request fails with `unable to open database file` (SQLite code 14). Both compose services set `ulimits.nofile: 65536`; a production deployment needs the same. It is also why collections are per kind of data, never per user.

- `Query` is approximate (HNSW). Real embeddings get near-perfect recall, but tests use tiny 2-D vectors, where a record written in a random order (e.g. concurrently) can be stored yet never returned. Tests that check what was written should count through the API (`chromaCount`), not through `Query`; query tests write sequentially and keep record counts small.

- A collection shared by several users must be filtered by owner on every `Query` (owner in `Metadata` and in `Where`). `Delete` with no IDs and `DeleteWhere` with an empty filter never reach the server, so a missing filter cannot wipe a collection.

- `domain.LanguageModel` is bound in `infraSet` with no consumer yet, so Wire leaves `llm.NewGemini` out of `InitializeApp` and the app needs no Gemini key. Once a usecase takes it, the app calls `Models.Get` for `GEMINI_MODEL` at boot and refuses to start with a missing or rejected key or an unknown model.

- `llm.Gemini` uses the Interactions API (`client.Interactions.Create`), not the legacy `GenerateContent`, with `store=true`: Google keeps the conversation (55 days paid tier, 1 day free tier), so a turn sends only new input plus `PreviousID`, and thought signatures are handled server-side. An old `PreviousID` stops working after that. `System`, `Tools` and `ResponseSchema` are not inherited and must be sent on every call.

- One `Generate` is one model step. When `FunctionCalls` is non-empty the caller runs them and calls `Generate` again with `PreviousID` and `FunctionResults`; there is no automatic loop.

- Function results go to Gemini as JSON text because the SDK's object form (`FunctionResultStepResult`) has no fields and would drop the result.

- Audio is sent inline only (base64). The raw total per request is capped at 14 MB so the request stays under Gemini's 20 MB limit. MIME parameters are stripped and common aliases (`audio/mp4`, `audio/x-wav`, ...) are mapped before checking the SDK's list.

- The Interactions client retries 408/409/429/5xx and connection errors up to `GEMINI_MAX_RETRIES`. A retried create can be billed and stored twice. `GEMINI_TIMEOUT` is the `http.Client` timeout, so it bounds each attempt, not the whole retry loop.

# muni-backend

Backend Muni — Go, Gin, PostgreSQL (pgx + sqlc), Google Wire, Swagger.

## Arsitektur

Clean architecture; dependency selalu mengarah ke dalam:

```
delivery/http ──► usecase ──► domain ◄── repository/postgres
                                  ▲
                                  └──── infrastructure (session token, bcrypt, db)
```

- `internal/domain` — entity, interface (repository, token, hasher), error. Tidak mengimpor layer lain.
- `internal/usecase` — logika aplikasi, hanya bergantung pada `domain`.
- `internal/repository`, `internal/infrastructure` — implementasi interface domain.
- `internal/delivery/http` — handler Gin, middleware, router, DTO.
- `internal/di` — wiring dependency dengan Google Wire (`wire.go` → `wire_gen.go`).

## Menjalankan

```bash
cp .env.example .env        # atur SESSION_TTL bila perlu
make docker-up              # Postgres + migrasi
make run                    # API di :8080
```

Swagger UI: http://localhost:8080/swagger/index.html

## Perintah

| Perintah | Fungsi |
|---|---|
| `make generate` | sqlc + swag + wire |
| `make migrate-up` / `migrate-down` | jalankan / rollback 1 migrasi |
| `make migrate-create name=xxx` | buat file migrasi baru |
| `make test` | jalankan Postgres test (Docker), seluruh test, lalu matikan DB |
| `make test-db-up` / `test-db-down` | nyalakan / matikan DB test saja |
| `make test-run` | jalankan test terhadap DB test yang sudah hidup (iterasi cepat) |
| `make lint` | go vet |

Setelah mengubah `db/queries/*.sql` atau skema migrasi jalankan `make sqlc`; setelah mengubah provider di `internal/di/wire.go` jalankan `make wire`; setelah mengubah anotasi handler jalankan `make swag`.

## Endpoint

| Method | Path | Auth |
|---|---|---|
| GET | `/health` | – |
| POST | `/api/v1/auth/register` | – |
| POST | `/api/v1/auth/login` | – |
| POST | `/api/v1/auth/logout` | Bearer session token |
| POST | `/api/v1/auth/logout-all` | Bearer session token |
| GET | `/api/v1/users/me` | Bearer session token |

Autentikasi bersifat stateful: `login` membuat baris di tabel `sessions` dan mengembalikan token opaque acak (hanya hash SHA-256-nya yang disimpan). Setiap request memeriksa sesi di DB, sehingga logout / revoke langsung berlaku. Masa berlaku diatur lewat `SESSION_TTL` (default `24h`).

## Testing

Test bersifat integrasi penuh: tanpa mock, berjalan di atas PostgreSQL asli (service `postgres-test` di `docker-compose.yml`, data di tmpfs, port `55432`, terpisah dari DB dev).

- `TestMain` menjalankan migrasi asli dari `db/migrations`, lalu membangun aplikasi lewat `di.InitializeApp` — graf Wire yang sama dengan produksi.
- Test ada di `internal/integration`; tabel di-`TRUNCATE` di awal tiap test.
- Port dapat diubah dengan `TEST_DB_PORT`; image dengan `POSTGRES_IMAGE`.
- Jika DB test belum hidup, test gagal dengan pesan yang menyarankan `make test-db-up` (tidak di-skip diam-diam).

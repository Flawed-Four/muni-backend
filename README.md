# muni-backend

Backend Muni — Go, Gin, PostgreSQL (pgx + sqlc), Google Wire, Swagger.

## Arsitektur

Clean architecture; dependency selalu mengarah ke dalam:

```
delivery/http ──► usecase ──► domain ◄── repository/postgres
                                  ▲
                                  └──── infrastructure (session token, bcrypt, db, smtp)
```

- `internal/domain` — entity, interface (repository, token, hasher, email), error. Tidak mengimpor layer lain.
- `internal/usecase` — logika aplikasi, hanya bergantung pada `domain`.
- `internal/repository`, `internal/infrastructure` — implementasi interface domain.
- `internal/delivery/http` — handler Gin, middleware, router, DTO.
- `internal/di` — wiring dependency dengan Google Wire (`wire.go` → `wire_gen.go`).

## Menjalankan

```bash
cp .env.example .env        # atur SESSION_TTL bila perlu
make docker-up              # Postgres + migrasi + Mailpit
make run                    # API di :8080
```

Swagger UI: http://localhost:8080/swagger/index.html

Mailpit UI: http://localhost:8025

## Perintah

| Perintah | Fungsi |
|---|---|
| `make generate` | sqlc + swag + wire |
| `make migrate-up` / `migrate-down` | jalankan / rollback 1 migrasi |
| `make migrate-create name=xxx` | buat file migrasi baru |
| `make test` | jalankan Postgres dan Mailpit test (Docker), seluruh test, lalu matikan keduanya |
| `make test-db-up` / `test-db-down` | nyalakan / matikan Postgres dan Mailpit test saja |
| `make test-run` | jalankan test terhadap service test yang sudah hidup (iterasi cepat) |
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

## Email

Pengiriman email lewat SMTP ada di `internal/infrastructure/email`, di belakang interface `domain.EmailSender`. Usecase yang perlu mengirim email cukup menerima interface itu sebagai dependency; implementasinya sudah terdaftar di Wire. Saat ini belum ada fitur yang memakainya.

Di lokal, `make docker-up` menyalakan Mailpit, yang menangkap semua email tanpa benar-benar mengirimkannya. Hasilnya bisa dilihat di http://localhost:8025.

| Env | Default | Keterangan |
|---|---|---|
| `SMTP_HOST` | `localhost` | |
| `SMTP_PORT` | `587` | `.env.example` memakai `1025` untuk Mailpit |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | kosong | Jika username kosong, koneksi tanpa auth |
| `SMTP_FROM_ADDRESS` | kosong | Wajib diisi saat email mulai dipakai |
| `SMTP_FROM_NAME` | `Muni` | |
| `SMTP_TLS_MODE` | `starttls` | `starttls` (port 587), `tls` (port 465), atau `none` (hanya untuk Mailpit) |
| `SMTP_TIMEOUT` | `10s` | |

Mode `starttls` gagal jika server tidak mendukung TLS; tidak ada fallback ke plaintext.

## Testing

Test bersifat integrasi penuh: tanpa mock, berjalan di atas PostgreSQL asli (service `postgres-test` di `docker-compose.yml`, data di tmpfs, port `55432`, terpisah dari DB dev) dan Mailpit asli (service `mailpit-test`, SMTP di port `51025`, API di `58025`).

- `TestMain` menjalankan migrasi asli dari `db/migrations`, lalu membangun aplikasi lewat `di.InitializeApp` — graf Wire yang sama dengan produksi.
- Test ada di `internal/integration`; tabel di-`TRUNCATE` di awal tiap test.
- Test email membaca hasil pengiriman lewat REST API Mailpit; inbox dikosongkan di awal tiap test.
- Port dapat diubah dengan `TEST_DB_PORT`, `TEST_SMTP_PORT`, dan `TEST_MAILPIT_PORT`; image Postgres dengan `POSTGRES_IMAGE`.
- Jika service test belum hidup, test gagal dengan pesan yang menyarankan `make test-db-up` (tidak di-skip diam-diam).

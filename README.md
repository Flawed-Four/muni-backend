# muni-backend

Backend Muni — Go, Gin, PostgreSQL (pgx + sqlc), Redis, ChromaDB, Google Wire, Swagger.

## Arsitektur

Clean architecture; dependency selalu mengarah ke dalam:

```
delivery/http ──► usecase ──► domain ◄── repository/postgres
                                  ▲
                                  └──── infrastructure (session token, bcrypt, db, smtp, redis, chroma)
```

- `internal/domain` — entity, interface (repository, token, hasher, email, key-value store, vector store), error. Tidak mengimpor layer lain.
- `internal/usecase` — logika aplikasi, hanya bergantung pada `domain`.
- `internal/repository`, `internal/infrastructure` — implementasi interface domain.
- `internal/delivery/http` — handler Gin, middleware, router, DTO.
- `internal/di` — wiring dependency dengan Google Wire (`wire.go` → `wire_gen.go`).

## Menjalankan

```bash
cp .env.example .env        # atur SESSION_TTL bila perlu
make docker-up              # Postgres + migrasi + Mailpit + Redis + Chroma
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
| `make test` | jalankan Postgres, Mailpit, Redis, dan Chroma test (Docker), seluruh test, lalu matikan semuanya |
| `make test-db-up` / `test-db-down` | nyalakan / matikan Postgres, Mailpit, Redis, dan Chroma test saja |
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

## Redis

Data berumur pendek yang dipakai bersama antar-instance (cache, OTP, rate limit) disimpan di Redis lewat interface `domain.KeyValueStore`, implementasinya ada di `internal/infrastructure/kvstore`. Usecase yang membutuhkannya cukup menerima interface itu sebagai dependency; implementasinya sudah terdaftar di Wire. Saat ini belum ada fitur yang memakainya, jadi aplikasi belum terhubung ke Redis. Begitu ada yang memakainya, aplikasi gagal start jika Redis tidak bisa dihubungi.

| Kebutuhan | Method |
|---|---|
| Cache | `Get`, `Set`, `Delete` |
| OTP | `Set` dan `GetDel` (kode sekali pakai), `Incr` (batas percobaan), `SetNX` (cooldown kirim ulang) |
| Rate limit | `Incr` (fixed window) |

Setiap key wajib punya TTL; TTL nol atau negatif ditolak. Nama key ditentukan pemanggil dan diberi namespace per fitur, misalnya `otp:login:<user id>`. Untuk kebutuhan di luar interface itu (pub/sub, stream), `*redis.Client` juga tersedia di Wire.

Di produksi, Redis dengan `maxmemory` harus memakai policy `noeviction`: OTP dan counter rate limit berbagi Redis dengan cache, sehingga policy yang mengevict bisa menghapusnya saat memori penuh.

| Env | Default | Keterangan |
|---|---|---|
| `REDIS_HOST` | `localhost` | |
| `REDIS_PORT` | `6379` | Juga port yang dibuka `make docker-up`; ubah jika 6379 sudah terpakai |
| `REDIS_USERNAME` / `REDIS_PASSWORD` | kosong | |
| `REDIS_DB` | `0` | |
| `REDIS_TLS` | `false` | Jika `true`, tidak ada fallback ke plaintext |
| `REDIS_TIMEOUT` | `3s` | Batas tiap dial, baca, dan tulis, juga ping saat start |

## ChromaDB

Embedding untuk pencarian semantik (misalnya mencari notes berdasarkan makna) disimpan di ChromaDB lewat interface `domain.VectorStore`. Implementasinya ada di `internal/infrastructure/vectorstore` dan berbicara langsung ke REST API v2 Chroma dengan `net/http`, tanpa client library. Usecase yang membutuhkannya cukup menerima interface itu sebagai dependency; implementasinya sudah terdaftar di Wire. Saat ini belum ada fitur yang memakainya, jadi aplikasi belum terhubung ke Chroma. Begitu ada yang memakainya, aplikasi gagal start jika Chroma tidak bisa dihubungi atau tenant/database-nya tidak ada.

| Method | Fungsi |
|---|---|
| `Upsert` | Simpan atau ganti record (ID, embedding, document, metadata) |
| `Query` | Cari `TopK` record terdekat dari sebuah embedding, bisa difilter dengan `Where` |
| `Delete` | Hapus record berdasarkan ID |
| `DeleteWhere` | Hapus semua record yang metadatanya cocok, misalnya semua potongan satu note |

Beberapa hal yang perlu diperhatikan:

- **Embedding datang dari pemanggil.** Chroma self-hosted tidak membuat embedding sendiri, jadi vektornya harus dihasilkan dulu oleh provider embedding (belum ada di proyek ini).
- **Distance memakai cosine**: `0` berarti arah sama, `1` ortogonal, `2` berlawanan.
- **Dimensi dan space terkunci per collection.** Collection dibuat oleh `Upsert` pertama, dan dimensi embedding-nya ikut terkunci saat itu. Ganti model embedding berarti pakai collection baru. `Query` dan `Delete` ke collection yang belum ada tidak membuatnya dan hanya mengembalikan hasil kosong.
- **Metadata di-merge saat upsert.** Key baru menimpa key lama, tapi key lama yang tidak dikirim tetap tersimpan. Embedding dan document selalu diganti.
- **Selalu filter berdasarkan pemilik.** Collection yang berisi data banyak user wajib menyimpan pemiliknya di metadata (misalnya `user_id`) dan memakainya di `Where` pada setiap query.
- `Where` hanya mendukung kesamaan nilai; jika ada beberapa key, semuanya harus cocok (AND). Tipe nilai: string, bool, integer, float.
- Satu request `Upsert` dibatasi `max_batch_size` server (5461 di Chroma 1.5.9 single-node, lihat `/api/v2/pre-flight-checks`).
- **Chroma butuh limit open files yang tinggi.** Ia membuka beberapa file per collection, termasuk collection yang sudah dihapus, sampai server di-restart. Dengan limit default 1024, Chroma mulai gagal (`unable to open database file`). Compose memakai `ulimits.nofile: 65536`, dan deployment produksi juga perlu. Karena alasan ini pula, buat collection per jenis data, bukan per user.

| Env | Default | Keterangan |
|---|---|---|
| `CHROMA_HOST` | `localhost` | Chroma Cloud: `api.trychroma.com` |
| `CHROMA_PORT` | `8000` | Juga port yang dibuka `make docker-up`; ubah jika 8000 sudah terpakai |
| `CHROMA_TLS` | `false` | Jika `true`, tidak ada fallback ke plaintext |
| `CHROMA_API_KEY` | kosong | Untuk Chroma Cloud, dikirim sebagai header `x-chroma-token`. Chroma self-hosted tidak punya auth bawaan |
| `CHROMA_TENANT` | `default_tenant` | |
| `CHROMA_DATABASE` | `default_database` | |
| `CHROMA_TIMEOUT` | `10s` | Batas tiap request, juga pengecekan saat start |

Chroma lokal dari `make docker-up` berjalan tanpa auth, jadi port-nya hanya dibuka di `127.0.0.1`. Datanya disimpan di volume `chromadata`. Image diambil dari `ghcr.io/chroma-core/chroma` dan bisa diganti lewat `CHROMA_IMAGE`.

## Testing

Test bersifat integrasi penuh: tanpa mock, berjalan di atas PostgreSQL asli (service `postgres-test` di `docker-compose.yml`, data di tmpfs, port `55432`, terpisah dari DB dev), Mailpit asli (service `mailpit-test`, SMTP di port `51025`, API di `58025`), Redis asli (service `redis-test`, port `56379`), dan Chroma asli (service `chroma-test`, data di tmpfs, port `58000`).

- `TestMain` menjalankan migrasi asli dari `db/migrations`, lalu membangun aplikasi lewat `di.InitializeApp` — graf Wire yang sama dengan produksi.
- Test ada di `internal/integration`; tabel di-`TRUNCATE` di awal tiap test.
- Test email membaca hasil pengiriman lewat REST API Mailpit; inbox dikosongkan di awal tiap test.
- Test Redis mengosongkan Redis test dengan `FLUSHDB` di awal tiap test.
- Test Chroma menghapus semua collection di database test di awal tiap test.
- Port dapat diubah dengan `TEST_DB_PORT`, `TEST_SMTP_PORT`, `TEST_MAILPIT_PORT`, `TEST_REDIS_PORT`, dan `TEST_CHROMA_PORT`; image dengan `POSTGRES_IMAGE`, `REDIS_IMAGE`, dan `CHROMA_IMAGE`.
- Jika service test belum hidup, test gagal dengan pesan yang menyarankan `make test-db-up` (tidak di-skip diam-diam).

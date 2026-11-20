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
- `internal/logging` — setup logger `slog` dan field per request lewat context (lihat [Logging](#logging)).

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

## Gemini (LLM)

Pemanggilan LLM lewat interface `domain.LanguageModel`, diimplementasikan di `internal/infrastructure/llm` dengan Interactions API dari SDK resmi `google.golang.org/genai`. Input bisa teks dan/atau audio, output berupa teks. Mendukung function calling dan structured output (JSON Schema). Model default `gemini-3.5-flash-lite` dan thinking `low`; keduanya bisa diganti per request lewat `LLMRequest.Model` dan `LLMRequest.Thinking`. Implementasinya sudah terdaftar di Wire, tetapi belum ada fitur yang memakainya. Begitu ada yang memakainya, aplikasi gagal start jika API key ditolak atau modelnya tidak ada.

Cara pakai dari usecase:

1. Panggil `Generate` dengan `Input` (teks/audio), dan bila perlu `System`, `Tools`, serta `ResponseSchema`.
2. Jika `FunctionCalls` tidak kosong, jalankan fungsinya, lalu panggil `Generate` lagi dengan `PreviousID` = `ID` response tadi dan hasilnya di `FunctionResults`. Ulangi sampai tidak ada function call.
3. Untuk turn berikutnya, cukup kirim input baru dengan `PreviousID`.

Beberapa hal yang perlu diperhatikan:

- **Percakapan disimpan di Google** (`store=true`): 55 hari di tier berbayar, 1 hari di tier gratis. Setelah itu `PreviousID` tidak berlaku lagi, jadi riwayat jangka panjang tetap harus disimpan sendiri.
- **`System`, `Tools`, dan `ResponseSchema` dikirim ulang di setiap panggilan**; nilai dari turn sebelumnya tidak terbawa.
- **Audio dikirim inline**, maksimal sekitar 14 MB data mentah per request (batas request Gemini 20 MB setelah base64). Format: wav, mp3, aiff, aac, ogg, flac, mpeg, m4a, l16, opus, alaw, mulaw, webm; parameter seperti `audio/webm;codecs=opus` dan nama alternatif seperti `audio/mp4` (Safari) diterima. Audio dihitung 32 token per detik.
- **Hasil fungsi dikirim sebagai teks JSON**, jadi `Result` boleh berupa nilai apa pun yang bisa di-`json.Marshal`.
- Request yang gagal karena 408/409/429/5xx diulang sampai `GEMINI_MAX_RETRIES` kali. Jika server sebenarnya sudah memproses request pertama, interaksinya bisa tercatat dua kali.

| Env | Default | Keterangan |
|---|---|---|
| `GEMINI_API_KEY` | kosong | Dari [Google AI Studio](https://aistudio.google.com/apikey) |
| `GEMINI_MODEL` | `gemini-3.5-flash-lite` | Model default, dicek saat start |
| `GEMINI_THINKING_LEVEL` | `low` | `minimal`, `low`, `medium`, atau `high` |
| `GEMINI_TIMEOUT` | `120s` | Batas tiap percobaan HTTP, juga pengecekan saat start |
| `GEMINI_MAX_RETRIES` | `2` | `0` mematikan retry |
| `GEMINI_BASE_URL` | kosong | Kosong berarti endpoint Google |

## Logging

Semua log ditulis lewat `log/slog` ke stdout. `main` memasang logger dari `internal/logging` sebagai default, jadi kode di layer mana pun cukup memanggil `slog.InfoContext(ctx, ...)` / `slog.ErrorContext(ctx, ...)`.

- Middleware `RequestLogger` memberi setiap request ID (`X-Request-ID` dari klien dipakai bila formatnya valid, selain itu dibuat baru) dan mengembalikannya di header response.
- Setiap request menghasilkan **satu** baris log `request` berisi `method`, `route`, `status`, `latency_ms`, `ip`, `request_id`, `user_id` (bila login), dan `err` (bila ada). Status 5xx dicatat sebagai `ERROR`, sisanya `INFO`. `/health` yang sukses tidak dicatat.
- Log yang ditulis dengan context request otomatis membawa `request_id` dan `user_id`, sehingga bisa dicari bersama di log aggregator.
- Panic dicatat sebagai `panic recovered` beserta stack trace, lalu dijawab 500.
- Body request, header `Authorization`, token, password, dan API key tidak pernah dicatat; yang dicatat adalah pola route (`/api/v1/users/me`), bukan URL beserta query string.

| Env | Default | Keterangan |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, atau `error` |
| `LOG_FORMAT` | kosong | `json` atau `text`; kosong berarti `json` bila `APP_ENV=production`, selain itu `text` |

Aplikasi tidak mengirim log ke mana pun sendiri. Pengumpulan log terpusat (mis. Grafana Alloy → Loki, atau Cloud Logging) membaca JSON dari stdout container.

## Testing

Test bersifat integrasi penuh: tanpa mock, berjalan di atas PostgreSQL asli (service `postgres-test` di `docker-compose.yml`, data di tmpfs, port `55432`, terpisah dari DB dev), Mailpit asli (service `mailpit-test`, SMTP di port `51025`, API di `58025`), Redis asli (service `redis-test`, port `56379`), dan Chroma asli (service `chroma-test`, data di tmpfs, port `58000`).

- `TestMain` menjalankan migrasi asli dari `db/migrations`, lalu membangun aplikasi lewat `di.InitializeApp` — graf Wire yang sama dengan produksi.
- Test ada di `internal/integration`; tabel di-`TRUNCATE` di awal tiap test.
- Test email membaca hasil pengiriman lewat REST API Mailpit; inbox dikosongkan di awal tiap test.
- Test Redis mengosongkan Redis test dengan `FLUSHDB` di awal tiap test.
- Test Chroma menghapus semua collection di database test di awal tiap test.
- Test logging mengalihkan logger default ke buffer JSON selama test berjalan (`captureLogs`). Log test lain ditulis sebagai teks ke stderr dengan level `TEST_LOG_LEVEL` (default `info`).
- Port dapat diubah dengan `TEST_DB_PORT`, `TEST_SMTP_PORT`, `TEST_MAILPIT_PORT`, `TEST_REDIS_PORT`, dan `TEST_CHROMA_PORT`; image dengan `POSTGRES_IMAGE`, `REDIS_IMAGE`, dan `CHROMA_IMAGE`.
- Jika service test belum hidup, test gagal dengan pesan yang menyarankan `make test-db-up` (tidak di-skip diam-diam).
- Test Gemini memanggil API Gemini asli dan hanya jalan jika `TEST_GEMINI_API_KEY` diisi (model bisa diganti dengan `TEST_GEMINI_MODEL`); tanpa key, test itu di-skip. `GEMINI_API_KEY` dari `.env` sengaja tidak dipakai supaya test tidak memakai key produksi. Contoh: `TEST_GEMINI_API_KEY=... make test`.

# Memona

Memori proyek bersama untuk agen AI dan manusia: catatan Markdown berversi, dashboard admin, dan endpoint MCP `/mcp`. Fokus MVP adalah konteks, keputusan, handoff, serta skill yang dikurasi; bukan eksekutor skrip atau pemindai seluruh server.

## Status dan batas klaim

Versi awal telah dibuild dan diuji otomatis: test Go, race detector, go vet, serta smoke test binary lokal lulus. Rincian bukti dan batasnya ada di [laporan verifikasi](docs/verification.md). Tool backup/restore offline juga diuji dengan fixture sintetis; pemulihan data pengguna/cutover operasional tetap pending. Dashboard belum diuji secara visual pada browser nyata; integrasi sesi OpenCode/Hermes serta akses Cloudflare masih pending. Tidak ada klaim uji lima langkah harness nyata yang sudah lulus.

Tidak ada instalasi global, service, tunnel, DNS, atau konfigurasi harness pengguna yang diterapkan. Tidak ada data pribadi lama yang diimpor.

## Kontrak MVP

- Browser: login password, sesi cookie HttpOnly/SameSite Strict dan token CSRF. UI menampilkan/mencari catatan, graph dan riwayat paginasi, editor create/revise dengan konflik revisi, saran duplikasi, publish skill serta archive/restore. Penulisan/revisi konten juga tersedia melalui MCP; acceptance browser fitur baru masih pending.
- Agen: bearer API key unik yang dibuat/dicabut melalui **dashboard**, izin `read`, `write`, `archive`, serta cakupan slug proyek atau `*`.
- Catatan: revisi immutable di `data/notes/<id>/NNNNNN.md`; SQLite untuk autentikasi/audit, bukan satu-satunya sumber pengetahuan.
- Retry MCP: `operation_id` opsional pada save/handoff/revise/archive mengembalikan hasil commit awal tanpa menulis ulang. Namespace per key/tool, payload berbeda ditolak; retensi bersama histori. [Kontrak dan risiko rollback](docs/idempotency.md).
- Diagnostik: menu admin **Penyimpanan** dan CLI offline `inspect` memeriksa kapasitas/integritas Markdown secara read-only, tanpa cleanup atau perbaikan otomatis. `/healthz` tetap liveness publik minimal. [Kontrak dan batas](docs/storage-health.md).
- Ringkasan proyek diprioritaskan saat pengambilan konteks. Relasi `related_to`/`supersedes` dan backlink satu-hop mengikuti proyek/izin; informasi yang digantikan tetap tersedia untuk peninjauan. Dashboard dapat mengatur metadata dan mengedit isi dengan revisi immutable. Lihat [kontrak relasi dan kompatibilitas](docs/memory-relations.md).
- Arsip bukan hard delete. Riwayat tetap disimpan dan dapat ditinjau; pemulihan tersedia melalui dashboard admin dan membuat revisi baru, bukan mengedit file lama.
- Retrieval memakai substring-first dan ranking leksikal; context mixed maksimal dua handoff serta alasan pemilihan per ID. Label eksplisit `is_testing` dikecualikan dari context default, tanpa mengubah izin search/read. Tool `find_duplicate_candidates` memberi maksimal sepuluh metadata kandidat satu proyek tanpa merge.
- Skill dari agen selalu draft, lalu admin meninjau dan memublikasikan. Metadata opsional pemicu/versi/kompatibilitas/bukti test tersedia; klaim penulis bukan verifikasi otomatis. [Kompatibilitas dan rollback](docs/skill-metadata.md). Skill tersimpan sebagai memori `kind: "skill"`; `get_skill` mengembalikan objek catatan beserta teks prosedur. Tidak ada direktori `data/skills`, registri native otomatis atau katalog HTTP skill. [Ekspor bundle portabel dan verifier offline](docs/skill-bundles.md) tersedia untuk references/assets yang ditinjau; bukan instalasi, eksekusi atau discovery native terverifikasi.

Harness dapat membaca aturan langsung melalui `get_memory_guide` atau resource `memona://guide` setelah server terbaru direstart. Lihat [panduan agen](docs/agent-guide.md).

Dashboard memakai **Svelte 5 + TypeScript + Vite**, dengan source terpisah di `frontend/`. Hasil build ditanam ke binary Go; Node tidak dibutuhkan saat runtime. Lihat [frontend dan alur build](docs/frontend.md), [arsitektur](docs/architecture.md), [keamanan](docs/security.md), [operasi](docs/operations.md), dan [integrasi serta acceptance](docs/integrations.md).

## Rencana pengembangan

Backlog berprioritas, checklist, dependensi dan kriteria selesai tersedia di [task pengembangan](docs/tasks.md). Daftar tersebut adalah rencana, bukan fitur yang sudah diimplementasikan atau izin deployment.

## Mulai lokal

Prasyarat: Linux/filesystem lokal yang mendukung `flock`, `fsync`, dan `renameat2(RENAME_NOREPLACE)`, Bash, Make, serta Go **1.26.8**. `go.mod` menyatakan baseline bahasa Go 1.26.0; toolchain 1.26.8 adalah target lingkungan, bukan klaim pin patch di direktif `go`. Dependensi dipatok: official MCP Go SDK **v1.8.0**, goldmark **v1.8.6**, modernc SQLite **v1.61.0**, dengan checksum di `go.sum`.

Untuk build UI terbaru diperlukan Node 22.18+ (lini 22) atau 24+ dan npm; disarankan Node 24 LTS. `make build` membangun frontend dahulu lalu binary Go. `make serve` menggunakan aset embed yang sudah ada, tanpa hot reload Svelte.

Jalankan dari root proyek. Buat konfigurasi hanya jika belum ada; jangan timpa konfigurasi pengguna:

```bash
cp -n config.example.json config.json
```

Periksa `config.json`, lalu bootstrap di terminal Bash interaktif (input tidak ditampilkan):

```bash
read -rs -p 'Password admin baru (minimal 16 karakter): ' MEMONA_ADMIN_PASSWORD
printf '\n'
export MEMONA_ADMIN_PASSWORD
make serve
unset MEMONA_ADMIN_PASSWORD
```

`unset` dijalankan setelah server berhenti; jangan aktifkan shell tracing (`set -x`). Password hanya untuk bootstrap admin pertama: minimal 16 karakter, tidak ada password default; database menyimpan hash dengan salt. Pada startup berikutnya admin yang sudah ada **tidak ditimpa** oleh variabel tersebut. `.env.example` hanya referensi; tidak ada janji aplikasi otomatis memuat `.env`.

Perintah aplikasi yang didukung:

```bash
go run ./cmd/memona serve -config config.json
go run ./cmd/memona inspect -data-dir DIREKTORI-YANG-SUDAH-ADA
```

`make serve` memakai perintah serve dengan cache proyek `.cache/`. `make build` menghasilkan `bin/memona`, kemudian `./bin/memona serve -config config.json`. `inspect` memerlukan direktori/lock yang sudah ada dan writer berhenti; jangan jalankan pada data aktif tanpa persetujuan downtime. Tidak ada perintah CLI reset password, backup, atau pembuatan key yang dijanjikan.

Buka `http://127.0.0.1:8765` pada mesin yang menjalankan aplikasi. Buat API key berbeda untuk OpenCode dan Hermes lewat dashboard; salin sekali ke secret environment harness, bukan ke chat, repository, atau log. Scope awal sebaiknya hanya proyek pengujian.

## Pengembangan dan produksi

```bash
make test   # go test ./...
make race   # go test -race ./...
make vet    # go vet ./...
make build  # npm ci → Svelte check/build → Go build
make check  # Go tests/race/vet + Svelte check + frontend/recovery unit tests
make recovery-test   # Unit test offline backup/verify/restore (fixture dipertahankan)
make recovery-smoke  # Binary yang sudah dibuild + restore sintetis terisolasi
```

Untuk pengujian binary terisolasi setelah build, jalankan `python3 scripts/smoke.py` (Python standard library). Script menyimpan fixture sintetis di `.cache/tmp/`, tidak mencetak secret, dan menghentikan server uji. Tidak ada cleanup otomatis. Hasil pengujian pengembangan tercatat di [verifikasi](docs/verification.md). Pengunduhan modul mungkin memerlukan jaringan; race memerlukan toolchain platform yang sesuai. Semua cache Makefile ada di `.cache/`; tidak ada target penghapusan otomatis.

Backup/verify/restore tersedia melalui `python3 -B scripts/recovery.py`, bukan CLI binary. Wajib stop normal sebelum backup, output baru, dan restore ke direktori terpisah. Baca [operasi](docs/operations.md) untuk perintah, batas keamanan dan risiko key/sesi kembali ke keadaan snapshot.

Untuk publik gunakan `public_url` HTTPS dan `secure_cookies: true`. Backend mendukung listener jaringan seperti `0.0.0.0:8765` di belakang reverse proxy TLS; default loopback tetap lebih terbatas. Lihat [akses jaringan](docs/network.md) dan `config.network.example.json`. Memona tidak melakukan terminasi TLS sendiri. Pemeriksaan host dan same-origin harus cocok dengan URL publik. **Minta persetujuan sebelum exposure/deployment**, termasuk pemasangan proxy, Cloudflare Tunnel/Access, perubahan firewall, atau DNS. Panduan [operasi](docs/operations.md) mencakup backup konsisten saat aplikasi berhenti dan restore ke direktori baru.

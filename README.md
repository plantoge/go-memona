# Memona

Memori proyek bersama untuk agen AI dan manusia: catatan Markdown berversi, dashboard admin, dan endpoint MCP `/mcp`. Fokus MVP adalah konteks, keputusan, handoff, serta skill yang dikurasi; bukan eksekutor skrip atau pemindai seluruh server.

## Status dan batas klaim

Versi awal telah dibuild dan diuji otomatis: test Go, race detector, go vet, serta smoke test binary lokal lulus. Rincian bukti dan batasnya ada di [laporan verifikasi](docs/verification.md). Tool backup/restore offline juga diuji dengan fixture sintetis; pemulihan data pengguna/cutover operasional tetap pending. Pengguna melaporkan uji browser dan alur OpenCode–dashboard–Hermes berhasil, tetapi versi/build, hasil per skenario, uji negatif dan FPS belum dicatat; ini bukan audit keamanan atau acceptance deployment publik. Akses HTTPS/reverse proxy dan instalasi service belum diuji pada home server.

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

### Instalasi baru di home server Linux (source → binary)

Ini **instalasi baru**, bukan migrasi dari server lama. Siapkan Git, Bash, Make, Python 3, Go 1.26.8, Node 22.18+ atau 24+ dan npm pada mesin build; gunakan akun Linux biasa yang akan memiliki direktori proyek dan data, **bukan root**. Pastikan filesystem lokal mendukung `flock`, `fsync`, dan `renameat2(RENAME_NOREPLACE)`; disk dan akses untuk mengunduh dependency harus cukup. Repo tidak menyediakan instalasi otomatis Go/Node, seeder admin, migration CLI, atau service. Node/npm hanya dibutuhkan ketika membangun UI; binary akhir sudah berisi aset dashboard.

1. Ambil source dari **URL repository dan branch/commit yang Anda percayai** (ganti placeholder, jangan menyalin URL contoh). Jika perubahan lokal di mesin pengembang belum di-commit dan di-push, clone tidak akan memuatnya:

   ```bash
   git clone <URL_REPOSITORY_ANDA> memona
   cd memona
   git log -1 --oneline
   make build
   ```

   `make build` menjalankan `npm ci --ignore-scripts`, pemeriksaan/build Svelte, lalu build Go ke `bin/memona`; build dapat mengunduh dependency. Untuk pemeriksaan tambahan jalankan `make test`, `make vet`, dan `make race`. Bila build gagal, jangan memakai binary lama sebagai bukti build baru berhasil. Semua perintah berikut dijalankan dari root proyek: `data_dir: ./data` relatif ke **working directory**, bukan lokasi file JSON.

2. Buat config awal tanpa menimpa config yang sudah ada. Pilih **salah satu** akses di bawah sebelum menjalankan server:

   ```bash
   umask 077
   cp -n config.example.json config.json
   ```

   - **Lokal/SSH tunnel:** biarkan `config.example.json` apa adanya (`127.0.0.1:8765`, URL `http://127.0.0.1:8765`, cookie non-Secure). Hanya bisa diakses langsung dari mesin server; untuk akses jarak jauh gunakan tunnel/proxy yang dikonfigurasi terpisah.
   - **Akses HTTPS home server:** bila reverse proxy TLS tepercaya berjalan **di mesin yang sama**, pertahankan `listen: "127.0.0.1:8765"` dan `data_dir: "./data"`, tetapi edit `config.json` menjadi `public_url: "https://NAMA_HOST_ASLI"`, `secure_cookies: true`, dan `allow_private_http: false` (atau hapus field terakhir). Atur reverse proxy secara terpisah agar meneruskan Host asli ke backend loopback, sertifikat valid, dan membatasi akses port backend. Ganti nama host dengan milik Anda; jangan memakai `NAMA_HOST_ASLI` secara literal. Memona sendiri hanya HTTP dan tidak mengelola sertifikat, DNS, firewall, atau proxy. Baca [akses jaringan](docs/network.md) dan [keamanan](docs/security.md) sebelum membuka akses publik.
   - **HTTP privat LAN/Tailscale khusus uji:** lihat [contoh dan risikonya](docs/network.md#testing-langsung-lantailscale). Jangan gunakan `allow_private_http` untuk internet publik, dan jangan port-forward backend HTTP.

   Jangan gunakan `0.0.0.0` tanpa kebutuhan dan pembatasan jaringan; binding wildcard dapat dijangkau interface lain. Config tidak otomatis diisi dari `.env`.

3. **Bootstrap admin sekali** di terminal interaktif milik akun pemilik data. Source Go membuat tabel SQLite, akun admin tunggal dan data Markdown saat startup; **tidak ada perintah seeder/migrate**. Password baru minimal 16 byte (gunakan minimal 16 karakter ASCII acak), maksimum 1024 byte, tanpa default. Input tidak tampak dan tidak dikirim sebagai argumen proses:

   ```bash
   read -rs -p 'Password admin baru: ' MEMONA_ADMIN_PASSWORD
   printf '\n'
   export MEMONA_ADMIN_PASSWORD
   ./bin/memona serve -config config.json
   unset MEMONA_ADMIN_PASSWORD
   ```

   Jangan aktifkan tracing shell (`set -x`); `unset` berjalan setelah server berhenti/command gagal. Jangan masukkan password ke chat, config, shell history, atau repository. Penggunaan environment hanya untuk bootstrap pertama; pada restart berikutnya admin yang sudah ada tidak ditimpa, dan password bootstrap tidak diperlukan. Jika database baru gagal bootstrap, **jangan hapus database/lock untuk mencoba reset**. Server berjalan di foreground; hentikan normal dengan `Ctrl+C`.

4. **Periksa akses.** Pada konfigurasi lokal, buka `http://127.0.0.1:8765` dari mesin server (atau melalui jalur yang sudah disetujui). Pada konfigurasi HTTPS, gunakan tepat origin `public_url`. Login sebagai administrator dengan password bootstrap; buat API key terpisah untuk OpenCode dan Hermes melalui menu **MCP Server**, batasi proyek/izin dan simpan token sekali ke secret environment klien. Jangan memakai password admin sebagai bearer `/mcp`. `/healthz` adalah liveness minimal, **bukan** pemeriksaan login/backup/penyimpanan. Uji juga login gagal, logout, scope key, dan akses browser sebelum membuka layanan ke pihak lain.

5. **Run rutin setelah bootstrap:** dari root proyek, `make status` lalu `make start` (atau `./bin/memona serve -config config.json`). Launcher `make start` tidak tersedia pada clone sampai perubahan lokal ini di-commit dan di-push; binary langsung tetap bekerja. `make start` membutuhkan data/lock yang sudah ada dan tidak membuat akun; tidak menjalankan build. Server foreground tidak otomatis hidup setelah reboot: jika perlu startup otomatis, rancang service non-root, secret/working directory dan graceful stop secara terpisah. **Tidak ada unit service terpasang oleh repo.** Jangan jalankan dua proses pada `data_dir` yang sama atau hapus `.writer.lock` untuk memaksa startup.

6. **Sebelum penggunaan rutin/upgrade:** lindungi direktori data, jadwalkan backup konsisten setelah stop normal, verifikasi digest dan latihan restore **ke direktori baru** sesuai [prosedur operasi](docs/operations.md). `git clone`/`git pull` tidak membawa atau memigrasikan `data/`, admin, key, maupun audit; `config.json`, `data/`, `bin/` juga tidak masuk Git. Untuk pindah mesin gunakan prosedur backup/verify/restore dengan pasangan binary/data kompatibel, jangan menyalin SQLite yang sedang ditulis atau menjalankan restore di atas data aktif. Cutover dan aktivasi HTTPS/service pada server nyata tetap perlu rencana uji dan rollback.

**Pemakaian harian setelah bootstrap:** dari root proyek, jalankan `make status` untuk memeriksa lock writer dan `/healthz` lokal, lalu `make start` untuk menjalankan binary yang sudah dibuild di foreground. Hentikan normal dengan Ctrl+C. `make start` tidak membangun ulang UI/Go, tidak membuat akun/data/config, dan menolak jika lock sedang dipakai; gunakan `make build` hanya bila source berubah. Pilihan lain tanpa Make: `python3 -B scripts/local.py status` / `python3 -B scripts/local.py start`. Berikan `CONFIG=path/yang/disetujui.json` pada Make bila bukan config default. Status tanpa dua bukti bukan readiness; lihat [operasi](docs/operations.md). Launcher bukan service/background manager dan tidak perlu Node saat runtime.

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

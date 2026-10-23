# Instruksi proyek Memona

## Ruang lingkup dan keamanan

- Kerjakan hanya tugas yang diminta di proyek ini. Baca source terkait sebelum mengubah dokumentasi/implementasi; jaga perubahan pengguna atau agen lain.
- Periksa status Git bila repository tersedia. Jangan mengklaim diff/commit atau test lulus tanpa hasil aktual.
- Jangan mengubah source dependensi, module cache, `go.mod`, atau `go.sum` untuk menyiasati error. Perubahan dependensi memerlukan kebutuhan yang jelas dan persetujuan sesuai kebijakan kerja.
- `AGENTS.md` milik SDK/dependensi di `.cache/mod/...` hanya berlaku untuk subtree dependensi tersebut; bukan instruksi proyek Memona dan tidak memperluas ruang lingkup tugas. Jangan memodifikasi SDK atau mengikuti workflow maintainer SDK sebagai workflow aplikasi.
- Jangan membaca/mengimpor data pribadi, memindai seluruh server, mencetak secret, atau mengubah konfigurasi harness/global. Exposure, service, tunnel, DNS, firewall, operasi destruktif, dan deployment memerlukan persetujuan.

## Toolchain dan cache

Target Go 1.26.8; ikuti pin di `go.mod`/`go.sum`. Direktif bahasa Go tidak selalu mematok patch toolchain. Gunakan target Makefile dari root proyek:

```bash
make test
make race
make vet
make build
```

Makefile menggunakan cache proyek absolut:

- `GOCACHE=$(CURDIR)/.cache/build`
- `GOMODCACHE=$(CURDIR)/.cache/mod`
- `GOPATH=$(CURDIR)/.cache/go`
- `GOTMPDIR=$(CURDIR)/.cache/tmp`

`make cache` menyiapkan direktori tersebut. Jangan mengganti cache dengan lokasi global, menjalankan `go env -w`, memasang software global, atau membersihkan cache/file pengguna otomatis. Untuk perintah Go langsung, pakai environment cache yang sama. Pengunduhan modul memerlukan akses jaringan; jangan mengklaim operasi sepenuhnya offline tanpa verifikasi. Binary aplikasi ada di `bin/memona`.

## Kontrak aplikasi saat ini

- CLI hanya `memona serve -config config.json`; jalur pengembangan `go run ./cmd/memona serve -config config.json` atau `make serve`. Jangan mengarang command reset/backup/key CLI.
- JSON runtime: `listen`, `data_dir`, `public_url`, `secure_cookies`. Listener dapat berupa IP loopback atau IP jaringan termasuk 0.0.0.0; listener nonloopback memakai public_url HTTPS dengan secure cookie/reverse proxy TLS, atau opsi eksplisit allow_private_http untuk testing LAN/Tailscale dengan URL IP privat dan cookie non-Secure. Jangan mengaktifkan opsi HTTP privat untuk internet publik. Memona sendiri belum melakukan terminasi TLS. Jangan melemahkan pemeriksaan host/origin/CSRF.
- Bootstrap password admin lewat environment, tanpa default. Admin lama tidak boleh ditimpa saat restart. API key dinamis dikelola via dashboard, bukan secret di contoh dokumentasi.
- UI Svelte + TypeScript + Vite di `frontend/` membaca/filter catatan dan histori, graph, editor create/revise admin+CSRF, diff teks serta publish skill/archive/restore. Konten juga dibuat/direvisi lewat MCP. Acceptance browser fitur baru masih pending. `make build` membangun UI dahulu, lalu embed `internal/webui/dist` ke binary Go. `make build-go`/`make serve` memakai dist yang sudah ada. Node hanya untuk development/build; baca `docs/frontend.md`. File UI vanilla lama dipertahankan sebagai referensi, tidak dilayani.
- Input mutasi memakai tepat `expected_revision`; hasil catatan memakai `revision`. `revise_memory` memerlukan seluruh field pengganti: `project`, `kind`, `title`, `content`, `status`, `source`, selain ID dan revisi.
- Key mempunyai izin `read`/`write`/`archive` serta scope proyek. Periksa scope asal dan tujuan revisi. Jangan menyamakan limiter global dengan kuota per-key.
- Markdown immutable di `data/notes/<id>/NNNNNN.md` adalah sumber pengetahuan. SQLite menyimpan auth, sesi, dan audit, bukan indeks pencarian. Pencarian masih scan notes dan substring lowercase.
- Skill adalah memori `kind: "skill"`, draft sebelum publish admin menjadi active. `get_skill` mengembalikan catatan/prosedur, bukan paket native; tidak ada `data/skills`, registry/katalog native otomatis, atau eksekusi skrip.
- Tidak ada hard delete. Restore arsip hanya melalui admin dashboard dan menambah revisi; tidak ada tool MCP restore. Jangan mengedit revisi lama, memaksa lock, atau menghapus berkas rusak untuk menyembunyikan error.
- Penulisan menggunakan staging dan rename atomik tanpa menimpa tujuan. Sisa staging tidak dibersihkan otomatis. Project/kind catatan tidak boleh berubah dalam revisi.

## Bukti dan pelaporan

Bedakan fitur yang terlihat dalam source, test otomatis yang benar-benar dijalankan, acceptance manual, dan deployment. Dokumentasi Bahasa Indonesia. Baca `docs/integrations.md` untuk skenario lintas OpenCode–UI–Hermes; jangan menyatakan lima langkah sudah lulus hanya dari review source. Laporkan file yang berubah, pemeriksaan yang dilakukan, gap, dan kebutuhan persetujuan. Backup konsisten memerlukan stop aplikasi dan restore ke direktori terpisah, bukan overwrite data aktif.

# Memona

Memona menyimpan memori proyek untuk manusia dan agen AI. Dashboard untuk admin; OpenCode/Hermes terhubung lewat API key. Panduan ini untuk **instalasi baru di Linux** (bukan pemindahan data lama).

## 1. Siapkan alat build

Pasang **Git, Go 1.26.8, Node.js 24 LTS (atau ≥22.18), npm, Make, Python 3, dan Bash** di home server. Gunakan akun Linux biasa, bukan root. Cek dengan `go version`, `node --version`, `npm --version`, `python3 --version`.

## 2. Ambil kode dan pasang dependensi

```bash
git clone URL_REPO_ANDA memona
cd memona
make build
```

Ganti `URL_REPO_ANDA` dengan alamat repo Anda. `make build` otomatis menjalankan **npm ci** untuk Svelte, pemeriksaan/build UI, mengunduh modul Go sesuai `go.mod`/`go.sum`, lalu membuat `bin/memona`. Perlu internet saat build pertama; Node/npm tidak dibutuhkan saat binary berjalan. Perubahan lokal yang belum di-commit dan di-push tidak ikut dalam clone.

## 3. Buat akun admin dan jalankan (pertama kali)

```bash
make setup
```

Perintah ini membuat `config.json` lokal hanya jika belum ada, meminta **password admin dua kali tanpa menampilkannya** (minimal 16 karakter ASCII), lalu menjalankan aplikasi di terminal. Tidak ada username terpisah, seeder, migrasi manual, atau password bawaan: Go membuat akun admin tunggal dan tabel SQLite saat startup pertama. `make setup` **menolak data yang sudah ada** dan tidak mereset password. Hentikan normal dengan **Ctrl+C**.

Buka **http://127.0.0.1:8765** di mesin server. Untuk mengakses dari komputer lain, gunakan SSH tunnel atau [proxy HTTPS yang dikonfigurasi terpisah](docs/network.md); default hanya mendengar di localhost. **Jangan port-forward HTTP langsung ke internet.**

## 4. Jalankan lagi

```bash
make start
```

Dari folder proyek, tanpa build/isi password lagi. `make status` untuk melihat indikasi proses aktif. Keduanya berjalan lokal; `make start` tetap di foreground dan tidak otomatis hidup setelah reboot. Alternatif tanpa launcher: `./bin/memona serve -config config.json`.

Di dashboard, buat API key berbeda untuk tiap agen melalui menu **MCP Server**; token tampil sekali. Jangan gunakan password admin sebagai API key.

**Sebelum dipakai sebagai layanan production:** atur HTTPS/reverse proxy dan cookie Secure, jalankan dengan akun non-root, uji login dan akses agen, rencanakan startup otomatis serta [backup/restore](docs/operations.md) dengan downtime. Repo belum memasang service atau proxy; `make start` saja **bukan deployment production otomatis**. `git clone` tidak membawa `data/` maupun akun admin dari server lain. Lihat [operasi](docs/operations.md), [keamanan](docs/security.md), dan [integrasi](docs/integrations.md) untuk langkah lanjutan.

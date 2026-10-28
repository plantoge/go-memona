# Memona

Memona menyimpan memori proyek untuk manusia dan agen AI. Dashboard admin dan ruang pengguna Google/team terpisah; OpenCode/Hermes terhubung lewat API key. Login Google hanya aktif bila kredensial OAuth dikonfigurasi. Lihat [akun dan team](docs/akun-team.md) sebelum mengaktifkannya. Panduan ini untuk **instalasi baru di Linux** (bukan pemindahan data lama).

Untuk agen coding, Memona dapat dipakai bersama Codebase Memory sebagai sumber struktur kode dan menyimpan rujukan kode pada `source` catatan. Indeks simbol Go lokal opsional tersedia melalui allowlist repository dan tool MCP `search_code_index`; lihat [batas dan konfigurasi indeks](docs/kode-repository.md). Tidak ada repository yang dipindai secara default.

## 1. Siapkan alat build

Pasang **Git, Go 1.26.8, Node.js 24 LTS (atau ≥22.18), npm, Make, Python 3, dan Bash** di home server. Gunakan akun Linux biasa, bukan root. Cek dengan `go version`, `node --version`, `npm --version`, `python3 --version`.

## 2. Ambil kode dan jalankan

```bash
git clone URL_REPO_ANDA memona
cd memona
make run
```

Ganti `URL_REPO_ANDA` dengan alamat repo Anda. **`make run` adalah satu perintah setelah clone:** memasang dependensi Svelte (`npm ci`), membangun Svelte+Go menjadi `bin/memona`, menyiapkan instalasi pertama, lalu menjalankan server. Perlu internet saat build pertama; Node/npm tidak dibutuhkan saat binary berjalan. Perubahan lokal yang belum di-commit dan di-push tidak ikut dalam clone.

## 3. Saat pertama kali

`make run` meminta pilihan **lokal**, **LAN/Tailscale privat**, atau **HTTPS lewat reverse proxy**, membuat `config.json` **hanya jika belum ada**, lalu meminta password admin dua kali tanpa menampilkannya (minimal 10 karakter ASCII; gunakan password acak yang lebih panjang bila memungkinkan). Go membuat akun admin tunggal dan tabel SQLite saat startup pertama; tidak ada seeder, migrasi manual, atau password bawaan. Hentikan normal dengan **Ctrl+C**.

Jika setup pertama gagal setelah membuat database tanpa admin, perbaiki penyebabnya lalu ulangi `make setup`: perintah ini membangun ulang Go sebelum meminta password, memeriksa bahwa tabel admin kosong, dan tidak mereset akun yang sudah ada. `make build` untuk memperbarui UI sekaligus Go.

Buka alamat yang ditampilkan di `config.json`: lokal **http://127.0.0.1:8765**; LAN/Tailscale memakai IP privat yang dimasukkan. HTTP privat hanya untuk jaringan tepercaya—**jangan port-forward ke internet**. Untuk domain HTTPS, Memona tetap di localhost; siapkan reverse proxy TLS (misalnya Nginx atau Caddy) dan sertifikat secara terpisah. Proxy **tidak wajib** untuk akses lokal/privat. Lihat [akses jaringan](docs/network.md).

## 4. Jalankan lagi tanpa build

```bash
make start
```

Dari folder proyek, tanpa build/isi password lagi. `make run` juga bisa dipakai lagi bila ingin **build ulang lalu start**, tanpa membuat ulang akun/config; menolak jika server masih aktif. `make status` untuk melihat indikasi proses aktif. `make start` tetap di foreground dan tidak otomatis hidup setelah reboot.

Di dashboard admin (`/admin`), buat API key berbeda untuk tiap agen lama melalui menu **MCP Server**; token tampil sekali. Pengguna Google (bila dikonfigurasi) masuk di `/` dan dapat membuat key untuk memori pribadi atau team. Jangan gunakan password admin sebagai API key.

Untuk mengaktifkan Google, isi `google_client_id` dan `google_client_secret` di `config.json` lokal, pastikan file privat (`0600`), dan cocokkan redirect URI dengan `public_url`; lihat [panduan akun/team](docs/akun-team.md). Jangan masukkan secret ke Git atau chat.

**Sebelum dipakai sebagai layanan production:** atur HTTPS/reverse proxy dan cookie Secure, jalankan dengan akun non-root, uji login dan akses agen, rencanakan startup otomatis serta [backup/restore](docs/operations.md) dengan downtime. Repo belum memasang service atau proxy; `make start` saja **bukan deployment production otomatis**. `git clone` tidak membawa `data/` maupun akun admin dari server lain. Lihat [operasi](docs/operations.md), [keamanan](docs/security.md), dan [integrasi](docs/integrations.md) untuk langkah lanjutan.

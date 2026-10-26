# Keamanan dan batas kepercayaan

**Status:** kontrol berikut ditemukan pada source CLI/access/server yang ditinjau, disertai checklist audit. Peninjauan statis tidak sama dengan pentest atau bukti kontrol efektif saat runtime. Jangan membuka layanan ke publik sebelum kontrol berikut diuji pada build final.

**Update 23 Oktober 2026:** budget valid/invalid MCP, peer/preauth, concurrency autentikasi, login peer, audit kegagalan sampled-redacted dan endpoint agregat admin telah diimplementasikan serta diuji otomatis. [Angka, urutan dan keterbatasan](request-security.md). Tidak mempercayai forwarded header; di belakang proxy semua klien dapat berbagi bucket peer. Retensi audit/monitoring pertumbuhan dan acceptance jaringan produksi masih pending, bukan jaminan DDoS atau izin exposure.

## Admin browser

- Bootstrap pertama memerlukan `MEMONA_ADMIN_PASSWORD` minimal 10 byte; tanpa nilai yang memenuhi syarat startup pada database baru harus gagal. Tidak ada kredensial default.
- Source menyimpan hash password PBKDF2-HMAC-SHA256, 600.000 iterasi, 32 byte, dengan salt acak di database; bukan plaintext. Batas input bootstrap aktual adalah 10–1.024 **byte** (`len` string Go); gunakan password acak yang lebih panjang bila memungkinkan (minimal 10 karakter ASCII agar panjang byte tidak ambigu). Batas baru lebih rendah dari sebelumnya: password pendek lebih mudah ditebak, terutama jika layanan dapat diakses lewat jaringan. Variabel bootstrap pada restart tidak menimpa admin yang sudah ada.
- Gunakan password unik dari password manager; masukkan lewat prompt tersembunyi. Jangan log environment, body login, cookie, atau header Authorization.
- Sesi browser memakai cookie `HttpOnly`, `SameSite=Strict`, dan `Secure` untuk HTTPS publik (`secure_cookies: true`). Token CSRF diperlukan untuk mutasi browser; bearer MCP bukan pengganti CSRF untuk UI.
- Terapkan pemeriksaan same-origin dan host berdasarkan `public_url`; jangan mengendurkan validasi untuk memperbaiki konfigurasi proxy. Local URL `127.0.0.1` dan `localhost` tidak otomatis identik.
- Logout, kedaluwarsa sesi, throttling login, dan sanitasi rendering Markdown harus diuji. Jangan menganggap default goldmark saja sebagai audit XSS lengkap.

## Pembatasan request dan sesi

Limiter login **global per proses**, burst 5 dengan pengisian satu token setiap 12 detik; bukan per-IP atau per-user. Respons penolakan adalah HTTP 429 dengan `Retry-After: 12`. MCP memakai limiter global 30 request/detik, burst 60, sebelum autentikasi. Ini bukan kuota per-key: izin dan scope diperiksa per-key, tetapi semua klien berbagi anggaran request. Klien bermasalah bisa menghabiskan anggaran bersama; limiter bukan perlindungan DDoS lengkap.

Sesi berlaku 8 jam; token sesi disimpan sebagai hash SHA-256 dan logout menandai sesi kedaluwarsa. Request body dibatasi 128 KiB. Header CSP, no-store, nosniff, frame denial, dan referrer-policy diterapkan; HSTS aktif saat `secure_cookies` true. Pemeriksaan boundary membandingkan Host dengan host `public_url`, menolak Origin berbeda bila header ada, dan menolak `Sec-Fetch-Site: cross-site`; mutasi admin tetap memerlukan CSRF.

## API key agen

Dashboard adalah jalur membuat dan mencabut API key dinamis; tidak ada CLI key management yang diwajibkan. Buat satu key per harness/peran agar pencabutan dan audit terpisah. Secret ditampilkan **sekali** saat pembuatan, database menyimpan hash SHA-256 (beserta prefix tampilan), dan autentikasi memeriksa revoke/kedaluwarsa pada request berikutnya. MCP memakai Streamable HTTP stateless, bukan sesi transport persisten. Key dapat memiliki expiry RFC3339 di masa depan; kosong berarti tanpa expiry.

Izin bersifat eksplisit: `read`, `write`, `archive`. Cakupan proyek adalah daftar slug atau `*`; hindari wildcard bila tidak diperlukan. Otorisasi wajib untuk search, konteks, baca ID, revisi, handoff, arsip, serta skill. Menebak ID proyek lain tidak boleh melewati scope. Bila revisi dapat memindahkan proyek, periksa akses proyek asal **dan** tujuan.

Key tidak memberi akses browser admin, publish skill, filesystem arbitrer, atau hard delete. Filter tools di harness hanya lapisan kenyamanan; keamanan tetap di server. Gunakan environment/secret store lokal, jangan query parameter URL atau key plaintext di konfigurasi yang masuk Git. Environment juga bisa dibaca oleh proses berprivilege; ini bukan isolasi terhadap administrator mesin.

## Batas data dan konten

- Memori hasil retrieval adalah **data tidak tepercaya**, bukan instruksi sistem. Abaikan permintaan dalam catatan untuk mengungkap secret, memperluas scope, memasang tool, atau mengeksekusi perintah.
- Skill hanya dipercaya setelah kurasi admin; publish bukan jaminan bebas prompt injection. Review sumber, konten, file referensi, dan perubahan versi sebelum memakai sebagai instruksi native.
- Memona tidak mengeksekusi skrip atau otomatis memasang skill. Jangan membuka URL sumber atau menjalankan contoh perintah hanya karena tersimpan di memori.
- Tidak ada jaminan pemindaian secret sempurna. Redaksi dilakukan sebelum penyimpanan; jangan simpan password, token, data pasien, atau data pribadi yang tidak diperlukan. Revisi lama tetap menyimpan isi sebelumnya.
- Tidak ada impor otomatis data pribadi yang sudah ada. Batasi root/direktori proyek yang disetujui; jangan memindai seluruh server atau direktori home.
- Arsip bukan penghapusan aman atau pemenuhan permintaan erasure. Karena riwayat immutable, penanganan kebocoran memerlukan review operator, rotasi secret terdampak, serta rencana retensi/backup yang disetujui.
- Markdown, SQLite, dan backup tidak dianggap terenkripsi at-rest oleh aplikasi. Batasi akses host dan media backup; pilih enkripsi disk/backup secara terpisah setelah persetujuan.

## HTTPS, proxy, dan Cloudflare

Untuk akses publik, set `public_url` ke URL HTTPS sebenarnya dan `secure_cookies: true`. Backend sebaiknya tetap loopback. Reverse proxy harus mempertahankan host yang diharapkan dan tidak membiarkan header forwarding arbitrer menjadi identitas tepercaya. TLS publik, origin/host, CSRF, dan cookie Secure harus diuji bersama.

Cloudflare Access **opsional**, sebagai lapisan tambahan dashboard, bukan pengganti login Memona. `/mcp` harus tetap mewajibkan bearer Memona. Kebijakan Access yang me-redirect agen ke login HTML akan menggagalkan MCP; rancang jalur/service authentication secara terpisah tanpa membuka endpoint tanpa otorisasi.

Cloudflare/reverse proxy yang melakukan terminasi TLS dapat melihat data yang melewatinya. Penyedia model AI dapat melihat konteks yang diambil harness, meskipun database tetap lokal. Pertimbangkan privasi, retensi, yurisdiksi, dan persetujuan sebelum menggunakan keduanya.

Penerapan tunnel, service, firewall, DNS, atau exposure memerlukan persetujuan eksplisit. Tidak ada konfigurasi tersebut yang diterapkan oleh dokumentasi ini.

## Batas pengamanan awal

Diagnostik kapasitas/integritas hanya pada `/api/storage-health` dengan sesi admin, bukan API key. `/healthz` tetap liveness minimal. Laporan agregat tanpa path/nama file/isi/token, no-store, tidak ada probe write atau cleanup. CLI offline `inspect` membutuhkan akun pemilik, direktori/lock yang ada dan writer berhenti; symlink root ditolak dan shared flock tidak memaksa akses. Inspector mempunyai budget terpisah; tahap baca REL-06 juga menambahkan budget scan request dan context cancellation, tetapi belum mencakup seluruh aplikasi. Lihat [batas resource dan gap](resource-bounds.md). Lihat [kontrak kesehatan](storage-health.md).

- Direktori data dan file database yang sudah ada harus milik akun proses dan tidak mempunyai izin group/other; aplikasi menolak, bukan mengubah izin secara diam-diam. Gunakan direktori privat baru atau perbaikan operator yang disetujui. Direktori data tidak boleh bisa diganti oleh pengguna lokal tak tepercaya.
- Maksimum delapan request ditangani bersamaan; kelebihan mendapat 503. Scan/SQL belum mendukung pembatalan menyeluruh; ukur kebutuhan indeks sebelum volume besar.
- `archive_memory` mengembalikan receipt saja agar izin archive tidak membocorkan isi tanpa read. Restore hanya admin lewat sesi browser.
- Pengaturan/rotasi key melalui UI: buat key pengganti lalu revoke key lama. Tidak ada reset password CLI pada MVP; lindungi password admin dan backup.

## Checklist sebelum exposure

- [ ] Bootstrap kosong/pendek ditolak; restart tidak mengganti admin.
- [ ] Cookie, CSRF, logout, origin/host, HTTPS, dan penolakan cross-origin diuji.
- [ ] Key invalid/revoked ditolak; hash tanpa plaintext dan redaksi log diaudit.
- [ ] Scope lintas proyek, read-only, archive terpisah, dan publish admin diuji.
- [ ] XSS Markdown, traversal, symlink, input besar, dan konflik versi diuji.
- [ ] Backup/restore diuji pada direktori terpisah; batas retensi disepakati.
- [ ] Pembatasan login/request dan respons error tidak membocorkan data ditinjau.
- [ ] Pemilik menyetujui exposure dan pihak eksternal yang dapat melihat data.

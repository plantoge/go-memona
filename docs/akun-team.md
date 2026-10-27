# Akun Google, memori pribadi, dan team

**Status:** implementasi kode dan test sintetis tersedia; login Google nyata, acceptance browser, serta aktivasi pada data aktif belum diverifikasi. Jangan membuka pendaftaran publik sebelum batas kapasitas dan pengamanan operasional disepakati.

## Model akses

- Satu identitas Google (`sub`, bukan alamat email) menjadi satu akun. Email terverifikasi digunakan untuk mencocokkan undangan, bukan sebagai ID akun; perubahan alamat email Google akan mengubah alamat yang dicocokkan.
- Setiap akun mendapat satu workspace pribadi. Pengguna dapat membuat beberapa team. Pembuat team adalah `owner`, anggota yang menerima undangan adalah `member`. Keduanya dapat membaca/menulis catatan team; hanya owner dapat membuat undangan. Pengguna dapat menjadi anggota beberapa team. Saat ini belum ada UI untuk keluar team, menghapus anggota, transfer owner, atau menghapus team.
- Undangan hanya muncul **di aplikasi** untuk akun Google yang masuk dengan alamat email yang diundang. Tidak ada email otomatis, masa berlaku undangan, atau pembatalan undangan saat ini. Pengguna yang belum pernah login dapat melihat undangan setelah login dengan email tersebut.
- Workspace mempunyai slug project internal `ws-<acak>` yang dipakai oleh tool MCP. Project adalah isolasi data; nama team hanya label. Key pengguna dibuat untuk tepat satu workspace dan hanya oleh anggota aktif. Key MCP diperiksa terhadap keanggotaan saat autentikasi, bukan hanya saat dibuat. Key pengguna mendukung izin `read`/`write`, belum `archive`; token ditampilkan sekali. Key bukan identitas login browser.
- `get_context` tanpa argumen project pada key pengguna memakai project workspace key tersebut. Pencarian tanpa project juga tetap hanya melihat project yang diizinkan. Untuk key lama, default `get_context` tetap `global`.
- Key lama, termasuk wildcard admin, tidak dapat membaca project dalam namespace `ws-`. Konsekuensinya, key lama untuk project historis bernama `ws-...` juga tidak dapat mengaksesnya lewat MCP. Admin lama tetap superadmin di API/dashboard admin dan dapat melihat data workspace; pemisahan halaman **bukan** pembatasan akses terhadap admin.
- Sesi pengguna (`memona_user_session`) dan admin (`memona_session`) terpisah. API pengguna memerlukan sesi pengguna dan CSRF untuk mutasi; API admin tetap memakai sesi admin dan CSRF. UI admin tersedia di `/admin` (juga `/` demi kompatibilitas); UI pengguna di `/user`. Semua endpoint user/MCP memeriksa scope server-side.

## Mengaktifkan login Google

1. Buat OAuth Client tipe **Web application** dan consent screen di Google Cloud Console. Daftarkan **Authorized redirect URI** yang persis sama dengan `<public_url>/auth/google/callback`; bila `tunnel_url` dikonfigurasi, gunakan `<tunnel_url>/auth/google/callback` dan buka login Google melalui hostname tunnel HTTPS, bukan alamat HTTP privat. Untuk penggunaan publik gunakan HTTPS dengan TLS reverse proxy dan `secure_cookies: true`; HTTP hanya untuk loopback development yang memenuhi validasi konfigurasi atau akses privat tanpa login Google melalui tunnel HTTPS. Memona tidak melakukan terminasi TLS sendiri.
2. Atur `google_client_id` dan `google_client_secret` dalam `config.json` lokal. Keduanya wajib disediakan bersama. Contoh bentuknya (placeholder **bukan** kredensial asli):

   ```json
   {
     "listen": "127.0.0.1:8765",
     "data_dir": "./data",
     "public_url": "https://memona.example.com",
     "secure_cookies": true,
     "google_client_id": "CLIENT_ID_ANDA",
     "google_client_secret": "CLIENT_SECRET_ANDA"
   }
   ```

   `config.json` di root proyek diabaikan Git, tetapi **harus berizin 0600 dan dimiliki akun yang menjalankan Memona** saat berisi kredensial; file symlink ditolak. Periksa izin file, lalu ubah secara sadar bila perlu (`chmod 600 config.json`), dan edit nilainya langsung di mesin Anda. Jangan tempel kredensial di chat, commit, log, atau contoh dokumentasi. Tanpa kedua field, login Google tidak aktif (`/auth/google/start` mengembalikan 404); satu field saja ditolak. Memona membaca file saat startup, jadi perubahan memerlukan restart terencana. `config.json` tidak dienkripsi: operator mesin dan backup masih harus melindunginya. Jangan gunakan kredensial nyata di lingkungan test bersama.
3. Jalankan build sesuai Makefile setelah meninjau perubahan dan backup konsisten. Memulai binary baru pada data aktif menambah tabel `users`, `user_sessions`, `workspaces`, `workspace_members`, `workspace_invitations` serta dua kolom nullable pada `api_keys`. **Jangan** aktifkan pada data aktif tanpa backup, uji pemulihan pada salinan terpisah, dan persetujuan operasional; tidak ada rollback schema otomatis. Memori Markdown lama tidak dipindah atau ditulis ulang. Database dan Markdown harus dipulihkan bersama dari snapshot konsisten, bukan menimpa data aktif secara parsial.
4. Kunjungi `/user`, pilih **Masuk dengan Google**, buat team, undang email anggota, lalu minta anggota menerima undangan di halaman team. Pada halaman kunci, pilih workspace, buat token, salin saat itu juga. Konfigurasikan klien MCP dengan URL `<public_url>/mcp` dan bearer token tersebut; gunakan project slug yang ditampilkan pada UI saat memanggil tool MCP.

Google OIDC menggunakan authorization code, state sekali pakai (masa berlaku lima menit), PKCE S256, nonce, verifikasi tanda tangan RS256/JWKS serta issuer/audience/waktu/email terverifikasi. Callback cross-site hanya diizinkan sebagai GET tanpa Origin dan tetap memerlukan cookie state yang cocok; API lain tetap memakai pemeriksaan Host/Origin/CSRF yang ada. Endpoint Google memakai HTTPS; alur live masih memerlukan pengujian dengan kredensial dan redirect URI operator.

## Batas versi ini

Ada batas awal **10 team yang dibuat per akun, 100 undangan pending per team, dan 100 key per akun (termasuk yang telah dicabut)**. Belum ada kuota total akun, catatan, ukuran konten, undangan yang sudah diterima, atau penyimpanan per team; audit/retensi tetap mengikuti kebijakan lama. Pendaftaran semua akun Google sesuai keputusan pengguna berarti beban penyimpanan dan penyalahgunaan masih dapat meningkat; **jangan menyatakan versi ini siap untuk signup publik/SaaS** tanpa kuota lebih lengkap, monitoring, backup, kebijakan retensi, dan uji keamanan serta browser nyata. Tidak ada deployment atau migrasi data aktif dalam pekerjaan ini. Scope team pada MCP tidak otomatis menggabungkan memori pribadi: tiap key menunjuk satu workspace. Fitur team belum meliputi pengelolaan anggota setelah menerima undangan atau email notifikasi.

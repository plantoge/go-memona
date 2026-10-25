# Operasi

## Lingkungan dan konfigurasi

Jalankan sebagai pengguna biasa dengan direktori proyek/data yang memang dimiliki pengguna tersebut. Jangan memasang software global, menggunakan sudo, atau mengubah service tanpa persetujuan. Target Linux + filesystem lokal dengan `flock`/`fsync`; bukan deployment multi-writer.

Salin `config.example.json` hanya bila belum ada (`cp -n config.example.json config.json`) lalu tinjau:

| Field | Lokal | Produksi yang disetujui |
| --- | --- | --- |
| `listen` | `127.0.0.1:8765` | Default loopback; IP jaringan/0.0.0.0 didukung dengan HTTPS proxy |
| `data_dir` | `./data` | Direktori khusus yang dibatasi aksesnya |
| `public_url` | `http://127.0.0.1:8765` | URL HTTPS sebenarnya |
| `secure_cookies` | `false` | `true` |

CLI membaca JSON melalui `-config`, menolak field tak dikenal, menerima listener IP loopback maupun jaringan (termasuk `0.0.0.0`; bukan hostname), dan mengubah `data_dir` relatif menjadi absolut berdasarkan working directory proses, bukan direktori file config. Listener nonloopback memakai `public_url` HTTPS dan reverse proxy TLS, atau `allow_private_http: true` untuk testing langsung IP LAN/Tailscale. Opsi HTTP privat default false; jangan gunakan untuk internet publik. Lihat [akses jaringan](network.md). `public_url` harus origin HTTP(S) tanpa kredensial/query/fragment/path aplikasi; HTTP hanya boleh untuk hostname loopback. HTTPS wajib `secure_cookies: true`.

Path relatif pada contoh dioperasikan dari root proyek. Jangan menambah field JSON rekaan untuk crawler/skill catalog. Proyek adalah scope logis; konfigurasi ini tidak memerintahkan pemindaian server. Gunakan hanya direktori data yang disetujui dan tidak berisi data pribadi lama.

Contoh produksi **untuk ditinjau, belum diterapkan**:

```json
{
  "listen": "127.0.0.1:8765",
  "data_dir": "./data",
  "public_url": "https://memona.example.com",
  "secure_cookies": true
}
```

Domain contoh harus diganti sebelum dipakai. Jangan membuka URL contoh atau membuat DNS/tunnel secara otomatis. Host/origin proxy harus cocok `public_url`; jangan mematikan pemeriksaan keamanan ketika menemui 403.

## Bootstrap dan menjalankan

Pada instalasi baru setelah `make build`, `make setup` (terminal interaktif) membuat config lokal jika belum ada, meminta password admin dua kali tanpa echo, lalu menjalankan binary di foreground. Data yang sudah ada ditolak oleh setup; jalankan `make start` untuk restart rutin. Bootstrap tetap dilakukan Go saat startup, bukan seeder akun default. Lihat langkah singkat pada [README](../README.md).

Setelah bootstrap pertama dan binary dibuild, alur rutin dari root proyek lebih singkat: `make status`, kemudian `make start` (foreground; Ctrl+C untuk stop). `make start` memakai binary `bin/memona` tanpa build/instalasi/penulisan konfigurasi, dan menolak bila direktori data/lock belum ada atau writer sedang aktif. `make status` memeriksa lock existing dan `/healthz` pada listener loopback; untuk bind wildcard hanya menghubungi loopback dengan Host dari public_url, **tidak** menghubungi URL publik. Lock tanpa HTTP sehat bukan bukti aplikasi siap, begitu pula HTTP tanpa lock data yang sama. Kedua indikator bersama pun tidak membuktikan identitas proses atau bahwa HTTP melayani data yang sama; periksa proses saat meragukan. Pada listener IP jaringan spesifik, probe jaringan dilewati dan status mungkin belum terkonfirmasi. Perintah ini tidak mengelola service, tidak memulihkan password, dan tidak menggantikan backup/monitoring. Untuk config berbeda gunakan `make status CONFIG=...` dan `make start CONFIG=...` setelah meninjau targetnya.

Ikuti prompt password di [README](../README.md). Bootstrap wajib pada database baru, minimum 16 karakter, tanpa default; hash bersalt disimpan, admin lama tidak ditimpa saat restart. Aplikasi tidak menjanjikan auto-load `.env`.

```bash
make build
./bin/memona serve -config config.json
```

Alternatif: `make serve`, yang menjalankan `go run ./cmd/memona serve -config config.json`. CLI menyediakan `serve` dan `inspect -data-dir DIREKTORI-YANG-SUDAH-ADA` untuk diagnostik read-only offline. Tidak ada perintah `memona reset`, `memona backup`, atau CLI generate key. Key dibuat/dicabut via dashboard. Jika password hilang, jangan menghapus database agar bootstrap ulang; hentikan perubahan dan susun pemulihan yang disetujui.

REL-05 memiliki [rancangan pemulihan admin lokal](admin-recovery-design.md), **belum implementasi CLI recovery**. Source/test sintetis telah disetujui, tetapi uji kelayakan menemukan URI descriptor SQLite dikonversi menjadi path biasa, sehingga belum memenuhi syarat pinning direktori/sidecar; implementasi reset dihentikan menunggu keputusan desain/refactor. Usulan mengubah hash, mencabut sesi dan menulis audit dalam satu transaksi offline; bukan cara reset lewat environment bootstrap. Reset data pengguna tetap memerlukan persetujuan tersendiri, tanpa penghapusan database atau penghentian layanan otomatis.

`Makefile` menggunakan `GOCACHE=$(CURDIR)/.cache/build`, `GOMODCACHE=$(CURDIR)/.cache/mod`, `GOPATH=$(CURDIR)/.cache/go`, dan `GOTMPDIR=$(CURDIR)/.cache/tmp`, bukan konfigurasi global. Path build/mod mengikuti cache proyek yang sudah tersedia. Binary berada di `bin/memona`. Go target 1.26.8 harus tersedia; tidak ada instalasi toolchain otomatis yang diklaim dalam panduan ini. Pengunduhan modul tetap bisa memerlukan jaringan.

## Backup konsisten: hentikan aplikasi lebih dahulu

Backup saat writer aktif dapat mencampur revisi Markdown dan transaksi SQLite. Jadwalkan downtime dengan pemilik. Hentikan proses secara normal (Ctrl+C untuk proses foreground), tunggu benar-benar keluar, dan pastikan tidak ada instance lain. Jangan menghapus lock atau menyalin hanya database tanpa berkas pendampingnya.

### Tooling offline yang direkomendasikan

`scripts/recovery.py` menyediakan `backup`, `verify`, dan `restore` memakai Python 3 standard library di Linux. Ini **bukan subcommand CLI memona**; tidak menjalankan atau menghentikan layanan. Jalankan sebagai pemilik data, dari working directory yang sama seperti server karena `data_dir` relatif memakai working directory. Pilih parent output milik sendiri yang tidak writable oleh pengguna lain; contoh memakai `backups/` lokal. Jangan menjalankan terhadap data aktif sebelum downtime disetujui.

```bash
mkdir -p backups
python3 -B scripts/recovery.py backup --stopped --config config.json --binary bin/memona --output backups/SNAPSHOT-BARU
```

Ganti `SNAPSHOT-BARU` dengan nama baru. `--stopped` adalah pengakuan bahwa aplikasi sudah dihentikan normal, bukan perintah stop. Tool juga mengambil lock writer eksklusif **yang sudah ada**, tanpa membuat/mengubah lock, dan menolak saat writer aktif. Seluruh data disalin termasuk revisi, staging tersisa, database/WAL/SHM bila ada; ditambah konfigurasi asli, binary yang diberikan, go.mod/go.sum, manifest versi tooling/format/waktu dan checksum SHA-256 tiap berkas. Pilih binary yang benar-benar kompatibel dengan format data snapshot, bukan mengasumsikan binary terbaru cocok dengan proses lama.

Output baru mode 0700 dan berkas 0600. Symlink (termasuk komponen parent), hardlink, device/FIFO, data berizin publik atau milik user lain ditolak, tanpa chmod sumber. Batas saat ini 100.000 entri, 16 GiB payload, 16 MiB manifest dan kedalaman 64 direktori; belum di-benchmark pada batas tersebut. Batas juga diperiksa saat copy/hash, bukan hanya saat prapemeriksaan. Inventory/hash sumber diperiksa sebelum/sesudah copy. **Tidak ada enkripsi, tanda tangan, upload, rotasi, atau cleanup otomatis.** Lindungi snapshot sebagai data sensitif. Tool menulis `manifest.json` terakhir lalu memverifikasi bundle sebelum melaporkan sukses; keluaran parsial dipertahankan dan tidak boleh dipakai ulang.

Catat digest manifest yang dicetak ke lokasi tepercaya terpisah. Verifikasi sebelum restore:

```bash
python3 -B scripts/recovery.py verify --backup backups/SNAPSHOT-BARU --manifest-sha256 DIGEST-YANG-DICATAT
python3 -B scripts/recovery.py restore --backup backups/SNAPSHOT-BARU --manifest-sha256 DIGEST-YANG-DICATAT --output restore-BARU --port 8766
```

Checksum membuktikan kesesuaian dengan manifest, bukan keaslian backup. Tanpa digest tepercaya, penyerang yang mengganti payload dan manifest tidak dapat dideteksi hanya dengan checksum internal. Tool tidak memeriksa kebenaran semantik semua catatan/SQL saat verify; startup salinan dan acceptance berikut tetap diperlukan.

Restore menolak tujuan yang sudah ada; tidak menimpa data aktif atau mengikuti path/link dari manifest. Seluruh checksum diverifikasi sebelum membuat output dan diverifikasi kembali sesudah copy. Salinan `config.original.json` hanya untuk referensi, **jangan jalankan dengan konfigurasi asli**. Tool membuat `config.restore.json` dengan data_dir absolut menunjuk salinan, listener/public_url loopback dan port alternatif, cookie non-Secure khusus uji lokal. Tidak mengeksekusi binary backup; berkas binary tetap non-executable. Setelah meninjau provenance/hash binary yang cocok, pengguna dapat menjalankan binary proyek yang telah diverifikasi dengan config restore tersebut. Jangan menyalakan restore publik atau menjalankan skrip/binary asing hanya karena checksum cocok.

Semua sesi dan key kembali ke keadaan waktu snapshot; sesi admin lama yang belum kedaluwarsa juga dapat kembali berlaku. **Key yang dicabut setelah snapshot dapat aktif kembali.** Rekonsiliasi pencabutan sebelum cutover; tool tidak mengubah autentikasi atau menghapus sesi otomatis. Cutover produksi dan penghentian proses pengguna memerlukan persetujuan tersendiri. Uji otomatis sintetis dijelaskan di [verifikasi](verification.md).

### Alternatif arsip manual

Contoh Bash berikut untuk konfigurasi `data_dir: ./data`, dari root proyek, **setelah aplikasi berhenti**. Ia membuat lokasi baru dan tidak menghapus data. Jika lokasi data berbeda, tinjau ulang daftar path sebelum menjalankan.

```bash
umask 077
mkdir -p backups
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="$(mktemp -d "backups/${stamp}-XXXXXX")"
go version > "$backup_dir/version.txt"
sha256sum bin/memona go.mod go.sum >> "$backup_dir/version.txt"
tar -czf "$backup_dir/memona.tar.gz" data config.json go.mod go.sum bin/memona
tar -tzf "$backup_dir/memona.tar.gz"
sha256sum "$backup_dir/memona.tar.gz" > "$backup_dir/memona.tar.gz.sha256"
```

Prasyarat contoh: binary yang **benar-benar dipakai** tersedia di `bin/memona`; jika menjalankan build lain, catat versi/hash artefak tersebut, jangan mengarsipkan binary lama. Tambahkan commit/release ID ke catatan versi jika tersedia. Periksa setiap exit status; jangan menganggap backup berhasil bila `tar`/checksum gagal. Arsip memuat seluruh data (termasuk SQLite/WAL/SHM bila ada), konfigurasi, dependensi, serta binary untuk identifikasi versi. Lindungi backup sebagai data sensitif; jangan unggah ke layanan luar tanpa persetujuan. Setelah backup tervalidasi, jalankan kembali dengan konfigurasi semula bila downtime berakhir.

## Restore ke direktori terpisah

Jangan ekstrak di atas data aktif. Pilih arsip terpercaya, cocokkan SHA-256 dengan manifest backup, dan periksa daftar anggotanya: tidak boleh ada path absolut, `..`, atau symlink yang keluar direktori tujuan. Ekstraksi arsip asing tidak aman.

```bash
archive="backups/REPLACE_WITH_BACKUP_DIRECTORY/memona.tar.gz"
tar -tzf "$archive"
restore_dir="$(mktemp -d "./restore-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
tar -xzf "$archive" -C "$restore_dir"
```

Ganti placeholder dengan lokasi yang telah diverifikasi sebelum menjalankan. Tinjau `config.json` di direktori restore: set `data_dir` ke salinan restore (bukan direktori produksi), port alternatif misalnya `127.0.0.1:8766`, dan `public_url` lokal yang cocok. Untuk uji lokal HTTP saja, `secure_cookies: false`; jangan membawa perubahan ini ke konfigurasi publik.

Jalankan binary yang cocok dari direktori restore hanya setelah review. Verifikasi login lama, catatan/riwayat, arsip, skill, scope, dan pencabutan key. Restore mengembalikan keadaan kredensial **pada waktu backup**: key yang dicabut kemudian mungkin aktif lagi dalam salinan; isolasi dari jaringan publik dan rekonsiliasi pencabutan sebelum cutover. Jangan menjalankan dua writer pada data yang sama. Perpindahan ke produksi memerlukan persetujuan dan rencana rollback dengan mempertahankan direktori asli; tidak ada penghapusan otomatis.

## Diagnosis aman

### Kesehatan penyimpanan REL-04

Menu admin **Penyimpanan** / `GET /api/storage-health` menampilkan kapasitas disk/inode, hitungan integritas Markdown, staging tingkat atas dan kegagalan write sejak proses mulai. Hanya cookie admin, bukan bearer MCP. `/healthz` tetap liveness publik minimal, bukan readiness/storage check. Scan read-only maksimal 20.000 entri/256 MiB/timeout kooperatif 5 detik; hasil terpotong bukan hasil bersih dan storage sibuk mendapat 503.

Jika `serve` gagal membuka histori rusak, setelah stop normal yang disetujui jalankan `./bin/memona inspect -data-dir DIREKTORI-YANG-SUDAH-ADA` sebagai pemilik data. Memakai shared lock yang sudah ada, menolak writer aktif, tanpa membuat/chmod/memperbaiki/menghapus file atau membuka SQLite. Exit 0 bersih, 2 JSON dengan warning/error/incomplete, 1 pemeriksaan tidak dapat dimulai/output gagal. [Arti field, kode, batas dan respons operator](storage-health.md).

### Aktivasi idempotensi REL-03

Field opsional `operation_id` pada empat mutation tool menyimpan receipt internal `_operation` dalam revisi Markdown, tanpa migrasi database atau perubahan revisi lama. Binary sebelum fitur ini dapat menolak revisi baru tersebut karena canonical encoding. Sebelum aktivasi yang disetujui, ambil backup konsisten binary/data yang sesuai; rollback memakai pasangan kompatibel ke direktori terpisah atau forward fix, bukan mengedit field histori. Restore menjaga bukti operasi sampai snapshot saja; rekonsiliasi operasi klien setelah snapshot sebelum retry/cutover. Tidak ada cleanup receipt/TTL. Lihat [kontrak idempotensi](idempotency.md).

### Kesalahan umum

- Gagal bootstrap: periksa panjang password tanpa mencetak nilainya. Jika admin sudah ada, variabel bootstrap bukan reset.
- Lock conflict: cari instance yang masih memakai data; jangan paksa hapus lock.
- Konflik `expected_revision`: baca ulang versi terbaru, review dan gabungkan secara eksplisit.
- Respons write hilang setelah memakai operation_id: ulangi ID/tool/key dan arguments asli. Konflik operation_id berarti payload berubah; jangan menaikkan expected_revision pada replay atau membuat ID baru tanpa memeriksa apakah operasi awal committed.
- 401/403 MCP: periksa header bearer, scope, revoke, dan proxy; jangan mencetak key.
- 403 UI/cookie hilang: periksa URL, HTTPS, host/origin dan CSRF; jangan menonaktifkan kontrol.
- Korupsi/partial write: hentikan writer, pertahankan semua file, buat salinan diagnostik, lalu rencanakan pemulihan.

## Verifikasi yang belum selesai

Test Go, race, vet, build serta smoke binary bootstrap/restart pernah lulus dalam pengembangan; lihat [laporan verifikasi](verification.md). Tool backup/restore offline telah diuji dengan data sintetis dan binary nyata di direktori terpisah. Backup/restore data pengguna, cutover operasional, uji browser dan kedua harness nyata masih pending. Panduan ini bukan laporan deployment atau jaminan kesiapan produksi.

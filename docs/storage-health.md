# Kesehatan penyimpanan (REL-04)

## Dua batas akses

- **Publik:** `GET /healthz` tetap menjawab `{"status":"ok"}` bila proses dapat menangani request. Ini **liveness**, bukan readiness disk/SQL dan bukan jaminan data sehat. Boundary Host/Origin/admission tetap berlaku.
- **Administrator:** menu **Penyimpanan** mengambil `GET /api/storage-health` dengan sesi cookie admin. API key MCP, termasuk scope `*`, tidak memberi akses. Hasil memakai `Cache-Control: no-store`; tidak ada tool MCP diagnostik atau parameter path. Sesi/Host/Origin/cross-site tetap diperiksa.

Laporan hanya mengandung waktu, hitungan, kapasitas, status, dan kode temuan. Tidak memuat nama file/path, ID/judul/isi/sumber/penulis catatan, payload staging, token atau pesan OS mentah. UI tidak menyimpan laporan di localStorage; laporan dibersihkan saat logout, berpindah menu atau refresh. Hasil lama tidak dianggap terkini setelah error. Tidak ada polling otomatis.

## Isi dan arti laporan

| Bagian | Arti dan batas |
| --- | --- |
| `disk` | `fstatfs` pada direktori data: total byte, byte tersedia bagi akun proses (`Bavail`), inode bebas. Bukan kuota spesifik pengguna/container, ukuran direktori atau probe write. |
| `disk.low` | Peringatan bila tersedia kurang dari 256 MiB **atau** 5% total, atau kurang dari 100 inode pada filesystem yang melaporkan inode. Ambang diagnostik tetap, bukan benchmark atau admission policy. |
| `integrity` | Scan direktori notes dan revisi: nama/urutan, ukuran/tipe/izin/pemilik, canonical Markdown, ID/revision, transisi histori, format receipt operasi dan scope receipt duplikat. Antar-catatan terbaru yang valid: target relasi seproyek/nonself, satu summary nonarsip, dan siklus supersedes termasuk catatan arsip. |
| `staging_entries` | Jumlah entri **tingkat atas** `.staging` yang teramati; tidak membuka payload atau menelusuri subtree/link. Selama scan online, mutex writer menjamin tidak menghitung staging yang sedang dikerjakan oleh Store. Entri ini bukti yang perlu dipertahankan, bukan otomatis kehilangan data atau aman dihapus. |
| `write_failures` | Counter in-memory sejak Store/proses mulai, waktu terakhir dan kode `storage_io`/`storage_corrupt` dari mutasi Markdown termasuk replay yang gagal fsync. Invalid input, expected_revision/operation conflict tidak dihitung. Tidak ada raw error, probe write, counter SQLite/audit/startup/Close, atau persistensi counter lintas restart. |
| `write_failures_observed` | `true` pada server, `false` untuk inspector offline yang tidak mengamati riwayat write proses sebelumnya. Nol bukan bukti disk dapat ditulis. |

`status` adalah `ok` bila scan selesai tanpa temuan/peringatan, `warning` untuk disk rendah/staging/riwayat write failure, dan `error` untuk temuan integritas/akses atau scan tidak lengkap. API tetap HTTP 200 bila laporan berhasil dibuat meskipun status error; baca fieldnya. Storage sibuk menghasilkan HTTP 503 + `Retry-After: 5`, bukan laporan bersih. Catatan/revisi dihitung dari yang diperiksa; revisi canonical masih mungkin memiliki masalah urutan/transisi. Hitungan masalah dapat tumpang tindih dan bukan jumlah file rusak yang unik.

## Read-only dan batas resource

Inspector tidak memanggil writer Open, membuat/chmod file, membuka SQLite, mengambil audit write, memperbaiki receipt/index persisted, fsync probe, menghapus staging, ataupun mengedit histori. Semua pembukaan memakai descriptor relatif dengan `O_NOFOLLOW`. FIFO/device/link pada jalur revisi ditolak tanpa dibaca; nama tak dikenal tidak diikuti. Seperti pembacaan biasa, filesystem dapat memperbarui access-time atau accounting internal; read-only berarti tidak ada mutasi byte/struktur/izin aplikasi, bukan snapshot filesystem forensik.

Scan memakai paling banyak **20.000 entri nama** (termasuk staging/note/revision yang dijumpai), **256 MiB ukuran revisi yang diperiksa**, dan pengecekan timeout kooperatif **5 detik**. Revisi dibatasi 1 MiB sebelum pembacaan. Pembatalan context diperiksa antarlangkah; syscall disk yang macet tidak dapat dihentikan paksa. Data lokal yang dikelola satu writer/akun tepercaya diasumsikan tidak dimutasi OS secara bersamaan. Batas ini bukan benchmark skala maksimum atau pembatalan menyeluruh seluruh aplikasi (REL-06 masih pending).

Pemindaian online memegang mutex Store agar writer/read lain tidak melihat view setengah berubah. TryLock menolak langsung bila mutex sedang dipakai; scan yang mulai lebih dahulu dapat menunda writer. Entri/byte/timeout yang membatasi pemeriksaan dilaporkan eksplisit dengan `complete:false`, bukan hasil bersih. Direktori tidak terbaca juga menandai incomplete. Kerusakan yang terdeteksi pada seluruh scan dapat memiliki `complete:true`; complete bukan berarti sehat. Target relasi yang tidak dapat diperiksa akibat catatan rusak juga bisa dilaporkan invalid_relation. Validasi lintas catatan memakai latest yang valid, bukan merekonstruksi seluruh graph pada setiap titik sejarah.

## Inspector lokal ketika server gagal startup

Binary kini menyediakan subcommand nyata berikut, tidak membaca config atau bootstrap password:

```bash
./bin/memona inspect -data-dir DIREKTORI-DATA-YANG-SUDAH-ADA
```

Ganti placeholder dengan direktori yang disetujui; path relatif berdasarkan working directory. Jalankan sebagai pemilik data. Memerlukan root/`notes`/`.staging` dan `.writer.lock` yang sudah ada. Tidak ada penciptaan direktori/lock atau perbaikan permission. Symlink pada komponen root ditolak. Mengambil shared flock nonblocking pada lock privat yang ada; writer eksklusif aktif membuat command menolak. Jangan menghapus/mengganti lock untuk memaksa pemeriksaan. Penghentian layanan pengguna memerlukan persetujuan dan stop normal; fitur ini tidak menghentikan proses.

- Exit **0**: laporan selesai tanpa temuan/peringatan.
- Exit **2**: JSON laporan tersedia, tetapi warning/error/incomplete perlu ditinjau (termasuk disk rendah/staging).
- Exit **1**: tidak dapat memulai pemeriksaan/argumen salah/lock sibuk atau output gagal; pesan generik di stderr, bukan path/OS error. JSON mungkin tidak lengkap bila output gagal.

Command dapat memeriksa revisi rusak yang menyebabkan `serve` gagal startup; jika struktur root/lock/direktori tidak dapat dibuka, ia menolak tanpa membuatnya. Jangan memakai inspector sebagai alasan membuka/membaca data pengguna tanpa izin. Berbagi laporan agregat tetap perlu pertimbangan informasi kapasitas/volume.

## Respons operator

1. Jangan retry write agresif, menghapus histori/staging/lock, atau menonaktifkan validasi.
2. Catat status/time/kode masalah tanpa konten. Konfirmasi downtime sebelum stop pengguna.
3. Pertahankan sumber; siapkan salinan konsisten dan binary kompatibel mengikuti [operasi](operations.md). Tool backup menolak link/tipe/permission berbahaya; jangan melemahkannya agar lolos. Kasus tersebut memerlukan penanganan operator tersendiri yang disetujui.
4. Periksa salinan dengan inspector/binary yang sesuai. Restore ke direktori baru, bukan overwrite aktif. Kerusakan semantik tidak diperbaiki otomatis oleh checksum backup.

**Di luar cakupan:** SQLite `integrity_check`, kesehatan auth/session/audit, validasi semantik/provenance isi, kemampuan write yang akan datang, pemantauan berkala, auto-alert, perbaikan otomatis, quota, readiness publik, dan jaminan filesystem/hardware. Health bukan pengganti [backup/restore](operations.md) atau acceptance browser/harness. Bukti aktual tersedia di [verifikasi](verification.md).

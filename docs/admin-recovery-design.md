# Rancangan pemulihan admin lokal — REL-05

**Status: implementasi source/test sintetis disetujui; tertahan pada pembuktian pengikatan path SQLite. Belum ada CLI recovery.** CLI saat ini hanya menyediakan `serve` dan `inspect`. Nama command di bawah adalah usulan, bukan command yang dapat dijalankan. Dokumen ini tidak mengizinkan reset, penghentian layanan, perubahan permission atau deployment.

## Tujuan dan batas

Memulihkan password admin yang hilang melalui akses OS lokal yang sah, tanpa menghapus database/memori, tanpa endpoint reset HTTP/MCP, dan tanpa mengubah perilaku bootstrap biasa. Password lama tidak diperlukan pada jalur recovery karena akun pemilik data adalah batas kepercayaannya. Ini adalah kemampuan sensitif: siapa pun yang menguasai akun OS pemilik data dapat mengambil alih admin. Tidak ada perlindungan terhadap administrator host atau proses lain dengan akses tulis setara.

Scope tahap implementasi yang diusulkan hanya source CLI/access, helper lease offline yang diperlukan, test sintetis, smoke terisolasi dan dokumentasi. Reset data pengguna, stop/restart layanan, aktivasi binary serta perubahan autentikasi pada instalasi aktif harus mendapatkan persetujuan tersendiri.

## Temuan dari source yang ada

- `internal/access/store.go`: `Open` membuat direktori/database bila belum ada, menjalankan DDL `CREATE TABLE IF NOT EXISTS`, mengatur WAL, dan bootstrap bila tabel admin kosong. Karena itu recovery **tidak boleh** memakai `access.Open` atau menjadikan password environment sebagai reset.
- Admin tunggal disimpan pada `admin(id=1,salt,hash)`. Hash saat ini PBKDF2-HMAC-SHA256, 600.000 iterasi, 32 byte, salt acak. Recovery tidak mengubah algoritma atau skema.
- Cookie admin diverifikasi melalui tabel `sessions`; `expires=0` menolak sesi pada request berikutnya. Tidak perlu menghapus baris untuk pencabutan.
- API key terpisah di tabel `api_keys`; reset password tidak perlu merotasi atau mencabutnya. Key yang bocor memerlukan operasi pencabutan terpisah.
- `memory.Open` mengambil lock writer eksklusif tetapi dapat membuat direktori/chmod lock dan gagal bila Markdown rusak. Recovery tidak boleh memanggilnya: kerusakan Markdown bukan alasan menulis ulang histori atau menggagalkan akses pemulihan autentikasi yang terkontrol.
- Inspector REL-04 membuka direktori dan lock existing tanpa follow symlink; pola akses tersebut relevan, tetapi shared lock inspector **tidak cukup** untuk reset. Recovery membutuhkan exclusive lock dan harus menolak inspector/backup/writer yang sedang memegang lock.
- `TestAdminBootstrapNotOverwritten` sudah menguji restart dengan environment bootstrap berbeda tidak mengganti password lama. Test ini wajib tetap berlaku.

## Antarmuka yang diusulkan

Usulan command: `memona recover-admin -data-dir DIREKTORI-YANG-SUDAH-ADA`.

1. Hanya operasi lokal offline, sebagai pemilik direktori/database. Tidak menerima config dengan lokasi data default yang dapat salah sasaran; data-dir harus eksplisit.
2. Buka terminal pengendali lokal, bukan pipe/stdin redirect. Tampilkan lokasi target yang sudah diperiksa kepada operator lokal agar tidak salah direktori; path tersebut tidak ditulis ke audit, output sukses, atau log diagnostik yang dibagikan. Render path secara escaped agar karakter kontrol tidak menipu terminal.
3. Tampilkan dampak: ganti password, cabut seluruh sesi browser admin, API key tidak berubah, tidak ada penghapusan memori. Minta frasa konfirmasi eksplisit; **tanpa** `--yes`/mode noninteraktif pada versi pertama. Konfirmasi runtime ini tidak menggantikan persetujuan operator atas downtime/reset data nyata.
4. Minta password baru dua kali tanpa echo. Terima 16–1.024 **byte** sesuai validasi panjang saat ini; input tidak dipangkas diam-diam. Batasi pembacaan sebelum mengalokasikan input tak terbatas; mismatch/EOF/Ctrl+C/input terlalu panjang membatalkan tanpa transaksi reset.
5. Jangan menerima password melalui argumen, flag, environment termasuk `MEMONA_ADMIN_PASSWORD`, file, clipboard otomatis atau log. Jangan mencetak password/hash/salt/token. Gunakan input byte mutable dan hapus buffer yang dikuasai secepat mungkin; jangan mengklaim penghapusan memori sempurna dari runtime/terminal/swap.
6. Pulihkan state terminal pada sukses/error dan interupsi yang tertangani. `SIGKILL`/terminal crash tidak dapat ditangani; dokumentasikan pemulihan echo manual di terminal operator, bukan menjalankan perubahan terminal global otomatis.

**Pilihan implementasi terminal:** evaluasi dukungan Linux `x/sys/unix` yang sudah dipin untuk echo/termios dan uji pseudo-terminal. Jika membutuhkan dependency terminal baru, ajukan kebutuhan serta perubahan module/lock terlebih dahulu; desain ini bukan persetujuan instalasi dependency.

## Pemeriksaan filesystem dan lease

Sebelum meminta password, validasi argumen dan struktur target tanpa mutasi:

- Tolak path kosong, traversal `..`, symlink pada seluruh komponen, tipe non-directory, serta root data yang bukan milik akun proses atau memiliki akses group/other. Tidak chmod/chown/create apa pun untuk memperbaikinya.
- Buka `.writer.lock` yang **sudah ada** dengan no-follow; verifikasi regular/private/owner/link count satu. Ambil `LOCK_EX | LOCK_NB`. Bila sibuk, berhenti dengan pesan aman; jangan mencoba shared lock, menghapus lock, memaksa unlock atau menghentikan proses.
- Pertahankan lease sampai koneksi SQLite benar-benar ditutup, termasuk saat commit/close error. Gunakan batas tunggu pendek, jangan menahan lock tanpa batas saat operator mengisi password. Urutan prompt dan lease harus diuji: rekomendasi dua fase validasi awal, input/konfirmasi, lalu pembukaan ulang dan validasi final dengan exclusive lease sebelum SQLite. Jika target berubah/busy saat fase final, batalkan dan jangan menerapkan reset.
- Verifikasi root/lock/database yang dipakai pada fase final identik dengan target yang dikonfirmasi (device/inode); jangan meneruskan input password ke direktori yang diganti.
- Database `memona.db` harus existing regular/private/owner/link count satu. Sidecar `-wal`/`-shm` bila ada harus diverifikasi private/owner/regular/no-follow/no-hardlink sebelum driver membukanya. Jangan menghapus sidecar atau menyalin database saja.
- Gunakan pembukaan SQLite **existing read-write**, bukan create/bootstrap/DDL. Cara mengikat driver SQLite beserta sidecar pada direktori yang dipin dan URI escaping wajib dibuktikan dengan test sebelum disetujui sebagai aman. Jangan mengeklaim descriptor file database saja mengamankan path sidecar; driver membuka file tambahan.

Asumsi akun OS pemilik dan parent data tidak dimutasi proses tak tepercaya tetap berlaku. Bila implementasi tidak dapat memenuhi batas path/sidecar tersebut tanpa refactor besar atau dependency baru, hentikan dan ajukan pilihan, bukan menurunkan pemeriksaan.

## Transaksi autentikasi

Setelah validasi final dan lease berhasil:

1. Buka satu koneksi SQLite dengan timeout terukur; tidak menjalankan schema migration atau PRAGMA yang mengubah mode jurnal yang sudah ada hanya untuk recovery.
2. Periksa schema yang didukung dan tepat satu admin `id=1`. Database kosong, versi tak didukung, tabel hilang atau gagal dibaca ditolak; **jangan** memperbaiki dengan bootstrap atau `CREATE TABLE`.
3. Buat salt acak dan hash baru menggunakan algoritma saat ini. Gagal RNG/hash membatalkan tanpa reset.
4. Dalam **satu transaksi**, update tepat satu baris admin; set `sessions.expires=0` untuk semua sesi browser; tambahkan event audit aman, misalnya actor `local-os:<numeric-uid>`, action `admin.password_recover`, target `admin:1`. Jangan masukkan path, alasan bebas pengguna, password, session token atau isi memori.
5. Jika update admin tidak tepat satu baris, pencabutan sesi gagal, audit gagal atau operasi SQL lain gagal, rollback keseluruhan. Audit recovery berbeda dari audit Markdown best-effort: untuk reset ini audit harus bagian transaksi yang sama.
6. Commit, tutup SQLite, baru lepas lease. Catatan Markdown, histori, receipt operasi, scope/key/hash/revoked dan konfigurasi tidak disentuh. SQLite dapat memperbarui WAL/SHM atau melakukan checkpoint sebagai bagian transaksi/close; jangan menjanjikan seluruh data byte-for-byte tetap sama.

Hasil sukses hanya setelah commit dan penutupan koneksi terkonfirmasi; output generik menyatakan password diperbarui, semua sesi admin dicabut, API key tidak berubah. Jika commit/close menghasilkan error yang membuat hasil ambigu, jangan menyatakan pasti rollback atau otomatis mengulang reset: laporkan kemungkinan commit dan verifikasi melalui login normal setelah restart yang disetujui. Crash setelah commit sebelum output tetap dapat berarti reset berhasil.

## Backup, operasi dan rollback

- Sebelum reset data pengguna, jadwalkan stop normal yang disetujui dan backup konsisten seluruh data beserta binary kompatibel mengikuti [operasi](operations.md). Recovery tidak membuat backup/stop/start otomatis.
- Untuk versi pertama, gunakan konfirmasi operator bahwa backup yang sesuai telah diperiksa; jangan menambahkan bypass diam-diam jika backup gagal.
- Password baru harus disimpan oleh operator di password manager, bukan dikirim ke chat. Jalur recovery tidak boleh mencoba login otomatis atau menampilkan token sesi setelah reset.
- Sesudah reset, restart normal hanya dengan persetujuan; buktikan login baru, password lama ditolak, sesi lama tidak berlaku, scope/revoke key tetap sama, serta catatan/histori tetap terbaca.
- Restore snapshot sebelum reset akan mengembalikan password/sesi/key ke keadaan snapshot. Pemulihan ke password lama juga dapat menghidupkan sesi/key yang sudah dicabut; lakukan restore ke direktori baru dan rekonsiliasi sebelum cutover, bukan menyalin tabel admin saja.
- Tidak ada endpoint jaringan baru, mekanisme reset publik, perubahan hashing, hard delete, pengurangan CSRF/Host/Origin/TLS, atau service/config global baru.

## Rencana uji wajib — belum dijalankan untuk fitur recovery

| Area | Bukti yang diperlukan pada fixture sintetis |
| --- | --- |
| Jalur sukses | Password baru diterima, lama ditolak, restart menjaga hash baru, audit aman tepat satu untuk satu commit. |
| Sesi dan key | Seluruh sesi lama ditolak melalui HTTP; valid/revoked/expired key dan scope identik; sesi baru setelah login normal berfungsi. |
| Atomisitas | Fault pada update session/audit dan commit sebelum selesai tidak meninggalkan reset setengah jalan; uji crash sebelum/sesudah commit serta output hilang tanpa janji exactly-once command. |
| Bootstrap | Test bootstrap lama tetap lulus; command menolak DB/admin/schema hilang dan tidak membuat database/tabel baru. |
| Lock | Server, backup, inspector atau recovery lain aktif ditolak; lock tidak dihapus/chmod; koneksi ditutup sebelum lease dilepas. |
| Filesystem | Parent/root/database/sidecar symlink, hardlink, FIFO/device, permission publik, owner salah dan pergantian target ditolak tanpa isi/path bocor di pesan error. |
| Terminal | Pseudo-terminal: echo off, dua input cocok, konfirmasi wajib, cancel/Ctrl+C/EOF/mismatch/oversize; terminal kembali normal. Non-TTY/pipe, password flags/env tidak menjadi jalan bypass. |
| Preservasi | Hash Markdown/revisi/staging/receipt/config dan record API key tidak berubah; backup/sumber pengguna tidak pernah dipakai sebagai fixture. |
| Operasional | Binary smoke loopback: stop sintetis, recovery, restart, login/session/key/audit/history; semua fixture dipertahankan tanpa cleanup. |
| Regresi | Go tests/race/vet dan check/build yang relevan; tidak mengubah test untuk menyembunyikan kegagalan atau memasang dependency tanpa review. |

## Hasil pemeriksaan kelayakan — 22 Oktober 2026

Persetujuan implementasi dan fixture sintetis sudah diberikan. Sebelum menambahkan kemampuan reset, `internal/access/recovery_path_test.go` menguji driver SQLite yang dipin, tanpa memodifikasi dependency:

- `TestRecoverySQLiteRWURIDoesNotCreateMissingDatabase`: URI `file:` yang di-escape dengan `mode=rw` menolak database hilang dan tidak membuat file. Ini memenuhi bagian existing-only, bukan seluruh kontrak filesystem.
- `TestRecoverySQLiteFDURICanonicalizesToPath`: URI `/proc/self/fd/<dirfd>/memona.db` dikonversi menjadi path biasa pada `PRAGMA database_list`. Sesudah direktori fixture di-rename, descriptor masih dapat menemukan database, tetapi nama yang disimpan SQLite menunjuk lokasi lama. Jadi shortcut URI descriptor **tidak membuktikan pinning direktori/sidecar**. Source VFS Unix dependency juga melakukan ekspansi symlink pada `xFullPathname`.
- Fixture memakai nama dengan karakter URI khusus setelah bootstrap pada path sederhana. Percobaan bootstrap langsung pada nama berisi `?` gagal; jalur bootstrap existing memakai DSN biasa. Perilaku startup tersebut tidak diperbaiki atau dipakai untuk melonggarkan kontrak recovery dalam tahap ini.

Dua test kelayakan dan regresi `make test`, `make race`, `make vet` lulus dengan Go 1.26.8, `GOPROXY=off`, `GOSUMDB=off` dan cache proyek. Log serta kegagalan probe awal tercatat di [verifikasi](verification.md). Ini **bukan** bukti transaksi reset, terminal atau recovery end-to-end; belum ada implementasi tersebut. Fixture retained di `.cache/tmp/admin-path-probe-*`; tidak memakai data pengguna.

**Blocker:** driver SQLite yang ada tidak menyediakan bukti bahwa shortcut URI descriptor mempertahankan pinning untuk pembukaan database dan sidecar. Pemeriksaan sebelum/sesudah open serta asumsi akun OS tepercaya tidak boleh diam-diam menggantikan syarat pinning pada rancangan ini. Implementasi reset dihentikan sesuai batas desain, bukan diteruskan memakai path biasa.

## Persetujuan yang diperlukan berikutnya

Pilih penanganan blocker: (1) kajian/refactor terpisah untuk VFS/akses filesystem SQLite yang benar-benar mengikat database dan sidecar pada direktori terverifikasi, tanpa mengubah module cache; atau (2) revisi kontrak desain secara eksplisit untuk menerima batas path-based dengan asumsi/riskonya, sebelum implementasi. Pilihan kedua bukan implementasi dengan jaminan setara. Alternatif paling aman adalah menunda REL-05 dan melanjutkan backlog non-autentikasi. Belum ada pilihan yang disetujui.

Persetujuan source/test yang sudah diberikan **bukan** izin reset pada data pengguna, downtime, aktivasi binary, instalasi dependency, ataupun konfigurasi sistem. Eksekusi operasional harus meminta persetujuan terpisah dengan target, risiko dan rencana pemulihan yang spesifik.

# Baseline version control dan preflight rilis — DEV01

## Audit terbatas saat penulisan

`git status --short` di root proyek mengembalikan `fatal: not a git repository` (exit 128). **Tidak menjalankan `git init`, staging, commit, push, atau mengubah `.gitignore`.** Tidak tersedia diff Git untuk pekerjaan ini. Audit hanya membaca `.gitignore` dan source terpilih; bukan pemindaian server, data aktif, credential, atau pembuktian bahwa seluruh source bebas secret.

Pola `.gitignore` yang ditinjau:

| Pola | Tujuan |
| --- | --- |
| `/.cache/`, `/bin/` | Cache toolchain/build dan binary lokal |
| `/data/`, `/backups/`, `/restore-*/` | Data, backup, dan restore di root yang cocok pola |
| `/config.json` | Konfigurasi runtime default |
| `/.env`, `/.env.*`, `!/.env.example` | File environment di root, kecuali contoh yang harus tetap nonsecret |
| `*.log` | Log |
| `/frontend/node_modules/`, `/frontend/.vite/` | Dependency/cache frontend |

Komentar secara sengaja **tidak** mengabaikan `internal/webui/dist`: aset ini merupakan input embed build Go-only. `go.mod`, `go.sum`, `frontend/package.json`, dan `frontend/package-lock.json` perlu baseline bersama source. Direktif bahasa `go 1.26.0` tidak membuktikan patch toolchain Go 1.26.8 yang benar-benar digunakan; catat versi toolchain dari proses build terpisah.

Batas audit: nama alternatif seperti `private-config.json`, direktori data/backup di lokasi lain, dump SQLite di root, secret di source, dan file environment bersarang tidak otomatis dilindungi pola berawalan `/`. Gitignore juga tidak mengeluarkan secret yang sudah pernah dilacak. Jangan menambah pola terlalu luas atau mengabaikan aset embed tanpa review. File contoh tetap perlu ditinjau nonsecret oleh pemiliknya, bukan dianggap aman karena ekstensi/nama.

## Langkah baseline yang belum dijalankan

1. Pemilik menentukan cakupan repository, kebijakan aset embed, lisensi, lokasi runtime/config/backup dan batas akses. Jangan menaruh credential/global harness atau data pasien dalam repository.
2. Setelah persetujuan inisialisasi Git, operator membuat repository lokal di root yang benar. Jangan memakai `git add .` atau `git add -A` sebelum review. Dokumen ini **tidak memberi izin** menjalankan langkah itu sekarang.
3. Review daftar calon file dari allowlist source yang eksplisit. Pemeriksaan ignore dapat memakai `git check-ignore --no-index` dengan daftar nama sintetis spesifik, tanpa membaca isi runtime. Review tambahan diperlukan untuk lokasi runtime nondefault. Tidak perlu menyapu semua server.
4. Stage hanya daftar source/contoh yang telah diperiksa. Review `git diff --cached --name-status`, lalu isi diff secara lokal oleh reviewer berwenang; jangan mengirim diff sensitif ke log/harness. Hindari merekam file nyata berisi key/config sekalipun nanti dihapus.
5. Jalankan test/build sesuai [verification.md](verification.md), simpan exit code dan hash binary nonsecret. Initial commit/tag memerlukan persetujuan terpisah. Remote/push/publikasi juga keputusan terpisah; jangan mengasumsikan persetujuan commit adalah persetujuan upload.
6. Setelah baseline disetujui, catat commit, dirty state, artefak dan toolchain. Bila ada secret terlanjur dilacak, hentikan publikasi dan rencanakan revoke/rotasi serta penanganan history dengan persetujuan; jangan melakukan rewrite destruktif otomatis.

## Preflight read-only yang dibatasi

Jalankan dari root source yang **secara eksplisit** dipilih:

```bash
python3 -B scripts/check_release.py --source-root /ABSOLUTE/REVIEWED_SOURCE_ROOT
```

Secara default script hanya membuka delapan file tetap berikut, tanpa recursive scan:

1. `.gitignore`
2. `go.mod`
3. `go.sum`
4. `Makefile`
5. `frontend/package.json`
6. `frontend/package-lock.json`
7. `cmd/memona/main.go`
8. `deploy/memona.service.example`

File harus regular, UTF-8, maksimal 4 MiB; symlink pada file/root/komponen induk ditolak. `bin/memona` hanya diperiksa metadata keberadaannya sebagai file regular nonkosong; bit executable dilaporkan, bukan dijadikan bukti bisa berjalan. Tanpa opsi tambahan, isi binary tidak dibaca. Root dan file source yang dipilih harus merupakan checkout tepercaya dan tidak diubah bersamaan; pemeriksaan symlink/identity bukan sandbox terhadap adversary yang mengganti komponen direktori saat pemeriksaan berjalan.

Hash binary hanya jika operator **secara eksplisit** memberikan path artefak binary yang boleh dibaca (bukan config/data/credential):

```bash
python3 -B scripts/check_release.py --source-root /ABSOLUTE/REVIEWED_SOURCE_ROOT --binary /ABSOLUTE/REVIEWED_ARTIFACT/memona
```

Binary opsional harus regular, nonsymlink, maksimal 512 MiB; tidak pernah dieksekusi. Jangan memberi path file sensitif sebagai `--binary`: opsi ini memang meminta membaca seluruh file untuk hash. Script tidak membaca environment, `.env`, JSON runtime, data, Git config, atau konfigurasi harness; tidak membuka network, menjalankan build/Git/binary, membuat file, atau mengubah service.

Output JSON memuat label pemeriksaan tetap, boolean, reason enum dan SHA-256; tidak memuat path input, isi file, pesan exception mentah, key atau password. Label `source_file_1`–`source_file_8` mengikuti urutan daftar di atas. Exit 0 berarti seluruh pemeriksaan terbatas lolos; 1 berarti ada temuan; 2 berarti argumen salah. Ini **bukan** sertifikasi keamanan/kelengkapan rilis, verifikasi semua dependency transitif, validasi sintaks unit, pembuktian binary berasal dari source, atau acceptance deployment.

Pemeriksaan pin: direktif Go dan dua dependency utama dibandingkan kontrak yang ditinjau; seluruh `devDependencies` frontend harus versi eksak dan cocok entri root lockfile. Hash manifest/sum dicatat untuk pembandingan antar-run; isi checksum/transitive resolution tidak dibuktikan oleh script. Pemeriksaan ignore hanya kecocokan pola yang ditentukan, bukan interpreter Git ignore lengkap atau secret scanner. Perubahan pin yang disengaja perlu review dan pembaruan kontrak/test, bukan melewati kegagalan tanpa alasan.

## Test sintetis

```bash
python3 -B -m unittest discover -s scripts -p test_check_release.py -v
```

Test membuat fixture sintetis di `/tmp/opencode/memona-preflight-*`; tidak menghapus fixture otomatis dan tidak membaca data/config proyek aktif. Hasil aktual dilaporkan oleh pelaksana, bukan diasumsikan oleh panduan ini. Menjalankan preflight tidak menggantikan test Go/frontend, browser, lintas harness, backup/restore maupun review [template service](service-template.md).

# Distribusi skill: ekspor portabel yang ditinjau operator

## Batas fitur saat ini

**Update 23 Oktober 2026:** exporter teks pada halaman ini tetap tidak menerima lampiran. Exporter terpisah `scripts/export_skill_bundle.py` mendukung input bundle references/assets eksplisit yang ditinjau; lihat [kontrak bundle](skill-bundles.md). Keduanya tidak memasang atau memverifikasi discovery native dan tidak mendukung script executable.

Memona menyimpan skill sebagai catatan `kind: "skill"`. Agen menulis draft;
admin meninjau/publish menjadi `active`. `get_skill` mengembalikan objek catatan,
bukan paket native, installer, registri, atau perintah eksekusi. Ekspor ini tidak
mengubah kontrak MCP atau lifecycle tersebut.

`scripts/export_skill.py` adalah alat **offline Python standard library**, bukan
subcommand binary Memona. Operator menyediakan satu file JSON eksplisit; alat
ini tidak menghubungi MCP, membaca Markdown aktif di `data/notes`, memindai
store, memeriksa status terbaru server, maupun membaca konfigurasi harness.
Tidak ada dependency tambahan, instalasi ke home/global, push, atau hook otomatis.

Hasil `SKILL.md` mempunyai frontmatter `name`/`description` bergaya format skill
native. Ini **ekspor portabel**, bukan bukti kompatibilitas atau auto-discovery
native OpenCode/Hermes. Folder hasil tidak langsung didaftarkan ke harness.
`native_discovery_verified: false` pada manifest disengaja dan bukan sakelar
untuk mengaktifkan integrasi.

## Kontrak input dan review

1. Ambil respons note lengkap dari `get_skill` melalui klien yang sudah
   diotorisasi. Simpan **objek note saja**, bukan envelope JSON-RPC atau teks
   Markdown, ke file baru milik operator dengan izin privat (misalnya `0600`).
   Jangan menyalin API key, secret, data pasien, atau percakapan lengkap.
2. Periksa ID, revision, status dan seluruh isi pada file yang persis akan
   diekspor. `active` bukan jaminan isi aman: JSON lokal bisa dipalsukan dan
   versi lokal bisa kedaluwarsa/diarsipkan setelah diambil. Tool ini tidak
   mengautentikasi provenance, mengecek ulang publish, atau merekam identitas
   reviewer. Bila perlu, cocokkan ulang dengan sumber berotorisasi secara manual.
3. Pastikan parent output sudah ada, milik operator, tidak writable oleh group/
   pengguna lain. Pilih **nama direktori output baru**, bukan direktori skill
   harness. Contoh di bawah memakai direktori proyek yang telah disiapkan;
   jangan menjadikan lokasi home/global atau direktori auto-discovery sebagai
   target. Alat tidak tahu semua lokasi auto-discovery khusus tiap harness.
4. Setelah review sadar terhadap file/revision tersebut, jalankan dari root
   proyek (path contoh harus diganti dengan path lokal yang sudah disiapkan):

   ```bash
   python3 scripts/export_skill.py --input operator-review/note.json --output operator-review/portable-skill-r7 --reviewed
   ```

`--reviewed` adalah pengakuan eksplisit operator, bukan pengganti peninjauan,
bukan verifikasi kriptografis, dan tidak boleh ditambahkan otomatis oleh hook.
Tidak ada opsi menjalankan isi, mengunduh link, atau menginstal hasil.

Contoh objek sintetis minimal:

```json
{
  "id": "0123456789abcdef0123456789abcdef",
  "revision": 7,
  "kind": "skill",
  "status": "active",
  "archived": false,
  "title": "Review data sintetis",
  "content": "Periksa data sintetis yang disetujui operator.\n"
}
```

Validasi sengaja lebih ketat daripada penyimpanan catatan umum:

- Wajib `kind: "skill"`, `status: "active"`, dan boolean `archived: false`.
- ID tepat 32 karakter heksadesimal lowercase; revision integer 1–999999,
  bukan boolean/float/string.
- Judul nonkosong, satu baris, maksimum 200 byte UTF-8. Konten nonkosong,
  maksimum 64 KiB UTF-8; tab/newline boleh, kontrol seperti NUL/ESC/CR ditolak.
- Input maksimum 128 KiB, JSON UTF-8 standar; duplicate key, NaN/Infinity,
  surrogate tidak valid pada judul/isi, envelope dan Markdown ditolak.
- Field `references`, `scripts`, `files`, `assets` ditolak meskipun kosong.
  Metadata note tambahan dapat hadir; `skill_metadata` divalidasi khusus (pemicu 1000 byte, versi 100, bukti test 2000, kompatibilitas maksimal 8 × 200 byte, total 4000). Pemicu nonkosong menjadi description; deklarasi metadata ikut manifest sebagai `declared_skill_metadata`, bukan bukti test atau kompatibilitas yang diperiksa otomatis. Link dan code fence
  dalam body tetap **teks tidak tepercaya**, tidak diikuti, dibaca, atau dijalankan.
  Referensi pendukung, skrip, aset, file samping, dan paket multi-file **belum
  didukung**; jangan menganggap link body telah dibundel atau divalidasi.

## Keamanan filesystem dan hasil

Tool menargetkan POSIX/Linux (`dir_fd`, `O_NOFOLLOW`, `O_DIRECTORY`). Input harus
regular file privat, dimiliki effective UID saat ini, dan mempunyai tepat satu
hard link. FIFO/device/directory, symlink file maupun ancestor, hardlink input,
serta komponen traversal `..` ditolak. Input dibuka read-only; metadata diperiksa
sebelum/sesudah pembacaan terbatas untuk mendeteksi perubahan selama baca.
Operator tetap harus mencegah penulis lain dengan UID yang sama memodifikasi
berkas; ini bukan sandbox terhadap proses bermusuhan dengan identitas sama.

Parent path dipin lewat descriptor tanpa mengikuti symlink. Parent output
harus milik UID saat ini dan tidak group/world-writable. Leaf output dibuat
baru `0700`; tujuan yang sudah ada, termasuk direktori kosong, file dan dangling
symlink, ditolak. File di dalamnya dibuat eksklusif `0600`, tidak executable:

- `SKILL.md`: nama deterministik berupa slug ASCII judul + suffix hash judul;
  description diturunkan dari pemicu penggunaan bila ada, atau judul, dan di-quote sebagai scalar JSON yang
  kompatibel YAML. Judul multiline ditolak agar tidak menyisipkan frontmatter.
  Body sumber ditaruh sesudah penutup frontmatter tanpa dieksekusi/disanitasi
  sebagai instruksi tepercaya.
- `manifest.json`: versi format/exporter, acknowledgement review, ID/revision
  sumber, SHA-256 byte input asli, jumlah byte/SHA-256 `SKILL.md`, dan batas fitur.
  Manifest ditulis terakhir; tidak memuat source path atau metadata sensitif lain.

SHA-256 membantu mencocokkan byte dan mendeteksi perubahan jika manifest/hash
pembanding dipercaya; **bukan signature atau bukti keaslian**. Jangan percaya
checksum yang ditulis ulang bersama payload. Manifest tidak memberi jaminan
bahwa native loader mengabaikan file lain atau menerapkan kebijakan keamanan.

Tidak ada overwrite maupun penghapusan otomatis. Kegagalan sesudah `mkdir`
dapat meninggalkan output parsial, termasuk manifest parsial ketika disk penuh.
Keberadaan manifest saja bukan bukti sukses: cek exit code dan checksum. Simpan
untuk diagnosis, pilih nama output baru untuk percobaan berikut; penghapusan
memerlukan keputusan operator tersendiri. Pesan CLI menyembunyikan path/isi
input untuk menghindari kebocoran tak sengaja.

## Verifikasi dan gap SKILL01

Jalankan pengujian sintetis dari root proyek:

```bash
PYTHONDONTWRITEBYTECODE=1 GOTMPDIR="$PWD/.cache/tmp" python3 -m unittest discover -s scripts -p 'test_export_skill.py' -v
```

Fixture dipertahankan di GOTMPDIR **di dalam proyek**, default `.cache/tmp`,
dengan prefix `skill-export-unit-`; tidak ada cleanup otomatis. Suite mencakup
review wajib, tipe/status/arsip/ID/revision tidak valid, batas byte, JSON berbahaya,
frontmatter, body tidak dieksekusi, traversal/symlink/hardlink/FIFO, izin/owner,
checksum/revision, input tidak berubah, output existing ditolak dan output
parsial dipertahankan. Ini bukan acceptance native atau penilaian keamanan isi.

**SKILL01 tetap pending** untuk distribusi native terverifikasi:

- Tetapkan kontrak metadata native yang disetujui (nama, description bermakna,
  kompatibilitas, versi, provenance/reviewer dan kebutuhan izin), bukan hanya
  schema note atau versi exporter.
- Catat versi konkret OpenCode/Hermes/OS dan format loader yang didukung;
  verifikasi dokumentasi resmi serta perilaku loader, jangan menganggap format
  frontmatter minimal cukup bagi semua versi.
- Uji manual pada sandbox proyek terisolasi: discovery tanpa instalasi global,
  load/selection sesuai nama/description, dukungan Unicode, penolakan metadata
  rusak, revisi kedaluwarsa/arsip dan prosedur upgrade/rollback tanpa overwrite.
- Rancang dan tinjau terpisah references/scripts/assets, trust boundary,
  checksum/signature serta izin eksekusi; exporter ini tidak mendukungnya.
- Rekam hasil pass/fail/blocked dan bukti versi aktual. Belum ada klaim
  auto-discovery, instalasi, eksekusi native atau acceptance lintas harness lulus.

Lihat juga [integrasi MCP](integrations.md) dan [kontrak hook manual](../integrations/hooks/README.md).

# Bundle skill portabel yang ditinjau operator

Status: implementasi offline Python standard library, 23 Oktober 2026. Bukan pemasangan atau discovery native OpenCode/Hermes. Tidak ada sinkronisasi otomatis, pengunduhan, pembacaan store aktif, atau eksekusi skrip.

## Input eksplisit

Operator menyiapkan direktori staging privat milik sendiri (0700), satu note JSON hasil review (0600), manifest input (0600), serta hanya file pendukung yang dinyatakan. Note harus `kind=skill`, `status=active`, `archived=false`, dengan ID dan revisi valid. Status dalam file adalah pernyataan snapshot, bukan pemeriksaan status terbaru server atau tanda tangan publikasi. Review seluruh isi, link, code fence dan gambar sebelum ekspor. Jangan memasukkan secret atau data pasien.

Contoh struktur staging:

```text
staging/
  bundle.json
  note.json
  references/
    guide.md
  assets/
    chart.png
```

`bundle.json`:

```json
{
  "format": "memona-skill-bundle-input-v1",
  "note": "note.json",
  "files": ["references/guide.md", "assets/chart.png"]
}
```

Nama note berupa basename JSON eksplisit; bukan URL atau path nested. `files` adalah path relatif terhadap staging sekaligus path tujuan, bukan mapping ke file server arbitrer. Tidak ada auto-discovery file atau scan rekursif.

```bash
python3 -B scripts/export_skill_bundle.py \
  --manifest /path/staging/bundle.json \
  --output /path/trusted-parent/new-bundle \
  --reviewed
```

Contoh bukan instruksi menjalankan terhadap data aktif. `--reviewed` wajib sebelum pembacaan apa pun. Output harus direktori baru di bawah parent yang sudah ada, milik operator dan tidak writable oleh orang lain. Direktori output yang sudah ada—termasuk kosong atau symlink—ditolak, tidak dilanjutkan/ditimpa.

## Path, format dan batas

- Ancestor dibuka descriptor-relative tanpa mengikuti symlink. Manifest, note dan lampiran dibaca melalui descriptor staging yang sama, walaupun nama direktori dipindahkan saat operasi. Direktori staging/anak harus privat, milik operator, tanpa special mode bits. File harus regular satu hardlink, privat, nonexecutable, tanpa special bits, milik operator; FIFO/device/symlink ditolak. Identitas/ukuran/waktu/mode/link diperiksa sebelum/sesudah read. Ini bukan snapshot OS atomik terhadap penulis lain dengan UID sama; operator harus menghentikan penyuntingan staging selama ekspor.
- Path hanya `references/...` atau `assets/...`, maksimal 8 komponen dan 240 karakter. Komponen ASCII aman maksimal 100 karakter, tidak hidden, tanpa traversal, backslash, komponen kosong atau trailing dot. Duplikasi, case collision pada file/direktori dan prefix file-versus-directory ditolak.
- Referensi: `.md`, `.txt`, `.json`, `.csv`, `.tsv`, UTF-8 tanpa karakter kontrol kecuali newline/CR/tab. Ekstensi bukan validasi format JSON/CSV.
- Asset: `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, dengan pemeriksaan signature sederhana. Bukan decoder gambar, antivirus, sanitasi polyglot atau jaminan gambar aman. SVG/HTML/script dan signature executable ditolak; kode di dalam teks tetap tidak dieksekusi.
- Maksimal **32 file output**, termasuk `SKILL.md` dan `manifest.json`: maksimal 30 lampiran. Maksimal **256 KiB per file**, **2 MiB total termasuk manifest**. Manifest input/note JSON masing-masing maksimal 128 KiB, isi note maksimal 64 KiB.
- Penolakan leksikal direktori store/harness/credential mengurangi salah target; bukan sandbox untuk UID sama atau bukti provenance. Input harus staging terisolasi, bukan salinan path yang dipilih tanpa review.

## Output dan kegagalan

`SKILL.md` memakai renderer exporter teks: frontmatter nama stabil dari judul dan description dari trigger metadata bila tersedia. Isi tetap teks tak tepercaya. `manifest.json` berformat `memona-portable-skill-bundle-v1`, menyimpan ID/revisi, SHA-256 note/manifest input, ukuran dan SHA-256 setiap payload, serta `native_discovery_verified=false`. Checksum membantu memeriksa salinan, bukan tanda tangan atau sertifikasi publikasi.

Direktori output baru 0700, file 0600/nonexecutable. Penulisan menggunakan create-exclusive dan fsync; manifest ditulis terakhir. Tidak ada cleanup otomatis, overwrite, chmod file lama, resume, instalasi ke home/global atau eksekusi. Kegagalan mempertahankan output parsial; jangan gunakan tanpa manifest dan pemeriksaan checksum. Jika penulisan manifest sendiri gagal, file manifest parsial pun mungkin ada—keberadaannya saja bukan bukti sukses. CLI melaporkan kegagalan generik tanpa mencetak path/isi input.

## Verifikasi offline sebelum penggunaan

```bash
python3 -B scripts/verify_skill_bundle.py --bundle /path/trusted-parent/new-bundle
```

Verifier read-only memvalidasi schema/versi manifest, inventaris tree tepat (tidak ada file/direktori tambahan), path dan collision, jumlah/byte, owner/mode/link, UTF-8 atau signature lampiran, serta checksum setiap payload. Manifest dibaca ulang dan inventaris diperiksa ulang sebelum berhasil. Tidak menulis, memperbaiki, menghapus, menginstal atau mengeksekusi isi. SKILL.md diperiksa struktur frontmatter dasar, bukan dievaluasi sebagai instruksi atau diklaim lolos parser harness. Sukses berarti salinan konsisten dengan manifest, **bukan bukti provenance, review yang benar, keamanan atau discovery native**. Penulis yang dapat mengubah payload sekaligus manifest dapat mengganti checksum; tidak ada tanda tangan. Ini juga bukan snapshot atomik terhadap proses UID sama yang mengubah tree selama verifikasi.

## Bukti dan gap

Test sintetis mencakup bundle nested, checksum/mode, acknowledgement sebelum read, schema/path/collision, batas file/bytes, symlink/hardlink/FIFO, owner/permission, signature executable/gambar, karakter kontrol, direktori staging berpindah, perubahan saat read, overwrite refusal, partial output dan redaksi CLI. Test tidak menguji decoder gambar, serangan kernel/filesystem, UID sama yang aktif menyerang, crash/power loss nyata atau discovery harness native.

SKILL-02 tetap parsial: scripts executable sengaja tidak didukung; penggunaan paket pada versi OpenCode dan Hermes terpasang, instalasi/discovery/update/rollback operasional memerlukan acceptance dan persetujuan terpisah.

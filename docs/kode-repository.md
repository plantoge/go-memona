# Bukti kode dan indeks repository

Memona menyimpan **keputusan/pengetahuan** dalam catatan yang ditulis manusia atau agen. Codebase Memory MCP mengindeks **struktur source** secara mandiri. Keduanya dapat dipakai bersama dalam klien MCP yang sama; Memona **tidak memasang, memanggil, atau menyinkronkan** layanan Codebase Memory dan tidak menyalin databasenya.

## Integrasi referensi (tanpa indeks native)

1. Dalam klien agen, cari simbol dan path melalui Codebase Memory (atau baca source langsung). Konfirmasi terhadap source/commit aktual; hasil indeks eksternal bisa tertinggal.
2. Simpan alasan dan keputusan di Memona sebagai `kind: decision` atau `knowledge`. Isi `source` dengan rujukan yang dapat ditinjau, misalnya `repo:go-memona; commit:<hash>; path:internal/server/mcp.go; simbol:mcpServerContext` atau URL permalink pada commit tertentu. Metadata opsional `code_references` juga dapat menyimpan `{ "repository": "go-memona", "path": "internal/server/mcp.go", "commit": "aabbccd", "symbol": "mcpServerContext", "line": 118 }`. Jangan menaruh token atau source rahasia dalam catatan.
3. Saat membaca ulang catatan, gunakan referensi itu untuk membuka source terkini. Perbedaan antara revisi lama dan kode terbaru harus ditinjau, bukan otomatis dianggap sudah sesuai.

Ini adalah integrasi **tingkat alur agen**, bukan sinkronisasi otomatis dua server MCP. Mengaktifkan koneksi Codebase Memory dalam klien agen dilakukan terpisah, dengan izin operator; tidak ada script instalasi yang dijalankan Memona.

`code_references` tersedia pada save/revise MCP dan API catatan pengguna; maksimum 10 rujukan per catatan, nama repo berupa slug, path relatif tanpa traversal, commit bila ada berupa 7–64 digit heksadesimal, simbol dan nomor baris opsional. Pada revisi, penghilangan field mempertahankan rujukan lama; array kosong menghapusnya. Catatan lama tetap dapat dibaca tanpa migrasi. Rujukan hanya metadata yang ditulis penulis: **tidak** memverifikasi commit/simbol, tidak membuka berkas, tidak menghubungkan project secara otomatis, dan tidak memberi izin tambahan. Jangan memakai rujukan itu sebagai bukti otorisasi terhadap repo lain.

## Indeks Go native (opsional, terbatas)

Operator dapat memasukkan repo lokal yang diizinkan dalam `config.json`:

```json
{
  "listen": "127.0.0.1:8765",
  "data_dir": "./data",
  "public_url": "http://127.0.0.1:8765",
  "secure_cookies": false,
  "code_repositories": [
    { "name": "aplikasi", "project": "contoh-proyek", "path": "/path/absolut/ke/repo-yang-diizinkan" }
  ]
}
```

**Jangan langsung menambahkan repo privat ke server aktif.** Pastikan repository itu memang boleh dibaca oleh semua pemegang key dengan izin `read` pada project yang dipetakan; untuk memori team, semua anggota yang mendapat izin baca team dapat mencari metadata kode repo tersebut. Periksa pula ukuran, pemilik, dan kandungan repository sebelum aktivasi. `path` harus absolut, direktori nyata tanpa symlink di komponennya. Tidak ada wildcard path, path dari agen, akses network, atau indeks otomatis ke semua repository. Jangan memakai `data_dir`, direktori rahasia atau repository dengan nama simbol sensitif sebagai target. Config di-restart secara terencana untuk memperbarui snapshot. Jika field dihilangkan, fitur indeks nonaktif.

Tool MCP `search_code_index` memerlukan `project` eksplisit dan izin `read` pada project itu. `query` adalah substring nama fungsi/method, path relatif, atau nama pemanggilan; `repository` opsional adalah nama dari allowlist, **bukan path**. Hasil mengandung path relatif, baris, nama, jenis simbol, dan hingga 40 **nama pemanggilan sintaksis** per fungsi. Ini **bukan** call graph yang di-resolve: nama selector dapat merujuk banyak paket dan tidak boleh dianggap edge pasti. Indeks hanya membaca berkas `.go` regular, tidak menyimpan isinya, mengabaikan direktori tersembunyi, `vendor`, `node_modules`, dan `testdata`; berkas Go yang gagal parse tidak menghasilkan simbol. Snapshot dibangun **saat startup**, tanpa watcher atau incremental index. Batas maksimum 16 repo, 30.000 entri direktori dan 2.000 berkas Go per repo, 8 MiB source per repo, 256 KiB per berkas, serta 20.000 simbol keseluruhan; limit yang terlampaui menggagalkan startup (bukan memindai tanpa batas). Hasil tool maksimal 50 simbol. `indexed_at` menunjukkan waktu snapshot, bukan waktu commit. Selalu periksa source terbaru untuk perubahan nyata.

Indeks native versi ini belum punya parser multi-bahasa, resolved dependency/call graph, pencarian semantik, analisis dampak perubahan, UI explorer, atau sinkronisasi langsung dengan Codebase Memory. Perubahan source baru terlihat setelah restart; refresh otomatis belum tersedia. Jangan gunakan hasil kosong sebagai bukti kode tidak ada. Pendaftaran repo di konfigurasi dan restart pada server aktif adalah keputusan operator; `code_references` tersimpan pada revisi Markdown immutable tanpa migrasi SQLite.

Acceptance sintetis tersedia di `scripts/smoke.py`: server dan repository percobaan di direktori sementara proyek, koneksi HTTP/MCP nyata ke binary lokal, isolasi key project lain, rujukan catatan, dan perubahan snapshot setelah restart. Ini **bukan** acceptance pada repository privat atau klien OpenCode/Hermes pengguna; jalur operator harus diuji terpisah setelah pemetaan project disetujui.

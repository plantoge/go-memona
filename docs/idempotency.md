# Idempotensi operasi tulis MCP

## Kontrak retry

`save_memory`, `save_handoff`, `revise_memory`, dan `archive_memory` menerima `operation_id` opsional. Discovery tetap sepuluh tool; enam field inti pada save/revise dan `expected_revision` pada revise/archive tetap wajib. Tidak ada header khusus atau alias JSON-RPC request ID untuk idempotensi. API dashboard admin belum mendukung identitas operasi.

- Format ID: 8–128 karakter ASCII, karakter pertama huruf/angka, sisanya huruf/angka/`.`/`_`/`:`/`-`. Gunakan ID acak unik per operasi (misalnya UUID), bukan token, informasi pribadi, atau tanggal saja. Hilang atau string kosong memakai perilaku lama **tanpa** deduplikasi retry.
- Namespace adalah **API key ID stabil + nama tool + operation_id**, bukan nama key atau proyek. ID yang sama pada key atau tool berbeda tidak berbagi hasil. Mengganti key saat retry bukan replay operasi awal dan dapat membuat catatan baru.
- ID dan argumen typed yang sama mengembalikan **revisi hasil commit awal**, termasuk ID/waktu/penulis yang sama. Tidak menambah revisi, walau catatan kemudian direvisi, diarsipkan, dipulihkan, atau skill dipublikasikan. Hasil ini bukan keadaan terbaru; gunakan read jika diizinkan.
- Payload berbeda dengan namespace/ID yang sama ditolak. Hash mencakup ID target, expected_revision, enam field konten, serta metadata opsional. Jangan mengubah expected_revision saat mengulang request yang hasilnya hilang; ID baru hanya untuk operasi baru setelah review.
- Urutan key JSON/whitespace transport tidak berpengaruh. Urutan array dan isi string berpengaruh; metadata yang tidak dikirim berbeda dari `false`/`[]`. Nilai null mengikuti decoding typed saat ini (misalnya null daftar menjadi nil); untuk kepastian pakai hilang atau nilai eksplisit, bukan null. Pada handoff, kind asli request ikut fingerprint walau hasil dipaksa handoff.
- Validasi/gagal konflik sebelum commit tidak mencadangkan ID. Jika error terjadi setelah rename, commit mungkin sudah terjadi: ulangi dengan **ID dan argumen asli**, bukan langsung mengganti ID. Tanpa operation_id, periksa keadaan sebelum mengulang write.

Contoh arguments (bukan envelope JSON-RPC):

```json
{
  "project": "acceptance-demo",
  "kind": "decision",
  "title": "Pilihan penyimpanan",
  "content": "Markdown tetap sumber pengetahuan.",
  "status": "unverified",
  "source": "",
  "operation_id": "7885ca90-6250-4771-8930-f29b17e4a60f"
}
```

Simpan arguments ini di state operasi harness yang aman sebelum mengirim. Bila timeout/response hilang, ulangi persis. Ini **bukan** izin retry tanpa batas; gunakan jeda dan laporkan kegagalan. Server tidak membuat ID otomatis untuk klien.

## Otorisasi dan hasil

Autentikasi, expiry/revoke, izin write/archive dan scope proyek tetap diperiksa pada setiap request **sebelum** lookup replay. Metadata masih memerlukan read+write. Revise memeriksa scope catatan asal dan proyek yang diminta. Replay tidak memberikan akses lintas key/proyek, tidak memberikan read pada key write-only, dan tidak melewati larangan metadata.

Respons archive tetap receipt ID/revision/archived saja. Respons revise write-only tetap menyembunyikan metadata lama yang dipertahankan. Metadata internal `_operation` tidak muncul pada output JSON MCP, API dashboard, context, atau history JSON. Audit mencatat request sukses termasuk replay; jumlah event audit bukan jumlah mutation unik. Penulisan audit SQLite bukan transaksi atomik bersama Markdown dan tidak dijamin exactly-once oleh fitur ini.

## Penyimpanan, crash, dan retensi

Bukti operasi v1 disimpan **di frontmatter revisi immutable yang melakukan commit**, dalam field storage-only `_operation` berisi version, hash scope, dan hash payload. Tidak ada tabel receipt SQLite terpisah, raw operation_id/key token, atau salinan payload kedua. Field tidak diwariskan ke revisi admin/legacy berikutnya. Catatan lama tanpa receipt tetap dibaca byte-for-byte tanpa migrasi.

Revisi dan bukti dipublikasikan bersama menggunakan staging + rename atomik + fsync yang sudah ada. Mutex writer mengurutkan lookup dan commit sehingga retry bersamaan hanya mempunyai satu commit. Saat startup, indeks in-memory scope → ID/revisi/payload dibangun ulang dari seluruh histori canonical Markdown. Receipt tidak valid atau duplikat dianggap korupsi, bukan dilewati/dihapus. Staging yang belum dipublikasikan bukan bukti commit dan tetap dipertahankan.

Jika operasi mengembalikan error setelah commit, indeks ditandai perlu dibangun ulang sebelum Execute berikutnya. Replay membaca revisi asli dan melakukan fsync direktori kembali sebelum melaporkan sukses. Test memodelkan celah post-rename dan proses keluar tanpa Close; ini bukan uji putus daya/perangkat keras yang menjamin filesystem apa pun.

Retensi **tanpa TTL**: bukti hidup selama revisi immutable. Arsip tidak melepas ID dan tidak ada cleanup/compaction otomatis. Indeks hanya memuat hash dan lokasi revisi, tetapi konsumsi RAM dan waktu startup tumbuh dengan jumlah operasi. Benchmark skala besar/batas kuota belum dikerjakan; lihat REL-06. Tidak ada cache kedua yang wajib dibackup.

## Backup, restore, dan rollback

Backup seluruh data mengikuti [operasi](operations.md); receipt otomatis ikut Markdown. Restore menjaga replay untuk operasi **yang sudah ada dalam snapshot**, dengan key/principal yang sama. Operasi setelah snapshot tidak tercatat pada salinan restore; retry operasi tersebut dapat menulis lagi. Cutover perlu rekonsiliasi state klien serta pencabutan key setelah snapshot.

**Binary sebelum REL-03 mungkin menolak canonical revision dengan `_operation`.** Jangan mengedit/hapus field atau revisi untuk membuatnya terbaca. Ambil backup konsisten beserta binary kompatibel sebelum aktivasi; rollback memakai pasangan binary/data kompatibel ke direktori baru, atau lakukan forward fix. Build/test ini tidak me-restart layanan pengguna atau memigrasikan data aktif. Harness perlu discovery/reconnect yang benar untuk mengenali operation_id.

## Bukti dan batas

Test storage/server memeriksa replay, konflik payload, namespace key/tool/proyek, omit/clear, empat mutation tool, konkurensi, restart, proses keluar mendadak, timeout setelah commit, staging, receipt rusak/duplikat, scope/permissions/revoke, serta redaksi write-only/archive-only. Runtime schema memverifikasi field operation_id opsional dan output create/replay tetap sesuai kontrak. Binary smoke dan recovery smoke sintetis membuktikan replay setelah restart/restore. Log/bukti dan gap acceptance nyata ada di [verifikasi](verification.md); bukan klaim acceptance browser/OpenCode/Hermes atau pemulihan data pengguna.

# RET-01 — batas rancangan indeks turunan

Status: kajian source 22 Oktober 2026, belum diimplementasikan. SQLite FTS5 tersedia pada probe terdahulu, tetapi bukan alasan menggabungkan indeks dengan database autentikasi.

## Keadaan sekarang

`ListContext` memvalidasi semua revisi canonical pada setiap snapshot. Test mengharuskan kerusakan pada revisi lama terdeteksi pada pembacaan berikutnya. Mutation staging/rename/fsync bersifat uninterrupted dan dapat menghasilkan commit ambigu. Receipt direkonstruksi dari seluruh histori dan replay mengembalikan revisi awal, bukan latest.

## Opsi yang dikaji

1. Indeks terpisah dengan snapshot latest di memori dan checkpoint turunan descriptor-relative. Startup tetap memvalidasi histori; mutation mengubah snapshot hanya setelah canonical commit. Hasil commit ambigu menandai indeks dirty, kemudian rebuild/fallback scan; kegagalan checkpoint tidak boleh mengubah successful canonical commit menjadi error biasa yang memicu duplicate retry.
2. SQLite/FTS terpisah. Memerlukan pembuktian filesystem database dan WAL/SHM, karena driver canonicalize URI descriptor menjadi path biasa. Jangan memakai auth DB atau menyamakan lstat + sql.Open dengan pinned openat.
3. Pertahankan scan canonical saat ini. Ini paling aman untuk kontrak korupsi sekarang, tetapi biaya tumbuh bersama histori.

## Keputusan yang belum boleh disamarkan

- Jika retrieval memakai cache tanpa scan, perubahan histori di luar aplikasi baru terdeteksi oleh startup/health/canonical read, tidak setiap search. Ini perubahan kontrak integritas yang memerlukan keputusan eksplisit, bukan optimasi transparan.
- Checkpoint disposable membutuhkan aturan lokasi privat, atomic replacement dan retensi. Tidak otomatis menghapus file rusak atau staging.
- Whole-snapshot checkpoint setiap write menambah O(total latest bytes) dan fsync; snapshot dalam RAM menambah baseline memory. Perlu benchmark sebelum aktivasi.
- Substring-first/partial identifier/Unicode/underscore tidak boleh berubah diam-diam karena FTS tokenizer berbeda.
- Snapshot lengkap berizin masih dibutuhkan context untuk overview/supersession dan relasi. Hit indeks sendiri bukan authoritative scope atau graph lengkap.

## Titik integrasi setelah keputusan

- `internal/memory/operations.go`: receipt rebuild startup dapat mengumpulkan latest pada pass validasi yang sama; publish hanya setelah lengkap. Jangan mengganti receipt reconstruction dengan latest-only.
- `internal/memory/store.go`: create/change canonical commit hooks, mencakup save/revise/archive/restore/publish. Pisahkan dirty receipt dan dirty retrieval.
- API baru berbeda dari `ListContext`, misalnya `LatestSnapshotContext`, agar kontrak scan lama/test korupsi tetap ada. Deep-copy relasi/skill metadata, cancellation dan budget eksplisit.
- `internal/server/connections.go`, dashboard/search/duplicates/graph memakai snapshot yang sama per request, tidak mencampur generation berbeda.

Tidak ada checkpoint, schema index, auth change, file replacement, migrasi atau aktivasi yang dilakukan dari dokumen ini. REL-05 juga tetap tertahan pada pinning SQLite. Permintaan pengerjaan menyeluruh bukan izin menurunkan kontrak integritas.

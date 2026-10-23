# Eksekusi backlog menyeluruh — 22 Oktober 2026

Pengguna meminta semua task dikerjakan. Pengerjaan source/test dibagi ke unit paralel dengan pemilik file terpisah; hasil tetap harus diverifikasi bersama. Ini tidak mengizinkan reset data aktif, perubahan permission/OS, instalasi dependency/layanan, deployment, exposure publik, migrasi PostgreSQL, penghapusan atau commit/push otomatis.

## Unit kerja

- REL-06: propagasi SQL context, audit bounded, cancellation admission mutasi tanpa memotong commit, cap JSON dan test.
- UI-01: backend create/revise melalui boundary admin+CSRF, form editor, diff teks revisi.
- UI-02: filter lokal arsip/status/penulis/tanggal/testing dengan batas hasil yang jelas.
- UI-03: endpoint graph terautentikasi, node mandiri, edge eksplisit, filter/zoom/pan/detail dan daftar aksesibel.
- UX-01: registry metadata proyek dengan slug stabil, optimistic revision, alias informasional, tanpa mengubah scope key atau migrasi catatan.
- RET-02/QUAL-01: ranking teks dan kandidat duplikasi bounded tanpa merge otomatis; probe FTS5 bukan indeks persisted yang sudah diimplementasikan.
- SEC-02: per-key rate limiter bounded, global limiter tetap; belum klaim audit/retensi kegagalan autentikasi lengkap.
- DEV-01/DEV-02/OPS-01: preflight release read-only, rancangan baseline Git, matriks acceptance, template layanan contoh (tidak dipasang).
- SKILL-02/AUTO-01: ekspor skill reviewed ke lokasi baru, tanpa instalasi/eksekusi; kontrak hook, bukan klaim auto-discovery kedua harness telah diuji.

## Batas yang tidak boleh disamarkan

REL-05 masih tertahan pada kontrak pinning SQLite dan memerlukan keputusan desain tersendiri. Tidak ada reset password nyata. RET-01 memerlukan desain konsistensi indeks dan pembuktian crash/rebuild; probe FTS5 saja tidak menyelesaikannya. RET-03 semantic search dievaluasi setelah dataset retrieval tersedia, bukan otomatis memasang embedding. Metadata verifikasi/skill yang mengubah format immutable perlu desain kompatibilitas sebelum ditambahkan. Wizard native/hook harus sesuai versi harness nyata; tidak mengarang konfigurasi yang belum diverifikasi. SEC-01 publik, reboot service, browser visual dan dua harness memerlukan lingkungan/persetujuan yang sesuai. Tidak menandai semua checkbox selesai dari implementasi sebagian atau unit test.

Bukti akhir dicatat di docs/verification.md dan ringkasan status di docs/tasks.md setelah pengujian aktual. Source proyek belum repository Git pada awal tahap ini; baseline diff tidak tersedia.

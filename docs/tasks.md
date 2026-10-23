# Task pengembangan Memona

Dibuat: 22 Oktober 2026.
Status: backlog; pengguna meminta pengerjaan menyeluruh pada source/test. Operasi aktif dan keputusan desain yang tertahan tetap memerlukan persetujuan spesifik.

## Checkpoint integrasi lanjutan

**Update 23 Oktober 2026 — metadata/retrieval/skill:** `is_testing` eksplisit tersedia pada penyimpanan immutable, MCP dan editor; revise omission mempertahankan, false membersihkan. Context default mengecualikannya tanpa mengubah hak search/read. `selection_reasons` menjelaskan setiap ID; batas 24000 byte sekarang menghitung seluruh JSON terstruktur termasuk guidance/alasan. Tool MCP read-only `find_duplicate_candidates` tersedia (11 tool total), berizin satu proyek, maksimal 10 metadata tanpa merge. Metadata skill opsional trigger/version/compatibility/test_evidence tersedia, preserve/clear dan receipt backward-compatible; deklarasi penulis bukan test otomatis. Editor/detail/diff metadata serta exporter teks terhubung. Graph diperbaiki untuk fokus/pointer/RAF/reduced-motion. Semua ini tetap source lokal, tidak diaktifkan pada layanan pengguna.

**Update bundle/resource/evaluasi:** exporter bundle offline baru mendukung references teks dan assets raster dengan input eksplisit, descriptor pinning, review wajib, checksum, output baru privat, batas 32 file/256KiB/2MiB dan tanpa scripts/install/overwrite/cleanup. 16 test bundle + 7 test verifier read-only + 47 test Python sebelumnya lulus (70 total). Verifier memeriksa tree tepat dan checksum tanpa repair/install. Benchmark 8 pembaca bersamaan dan profil histori lebih besar tercatat dalam [pengukuran](resource-benchmark-2026-10.md), bukan kapasitas maksimum/RSS/deadline keras. Evaluasi semantic sintetis menemukan 5/9 pasangan relevan terambil (fixture sengaja kecil, bukan korpus pengguna); embedding belum dipilih/dipanggil. Lihat [evaluasi](semantic-search-evaluation.md) dan [bundle](skill-bundles.md). RET-01/REL-05 tetap tertahan pada kontrak integritas/pinning; metadata waktu review QUAL-02, formal review state, acceptance browser/harness dan operasi aktif masih terbuka. Entri checkpoint di bawah bersifat historis bila bertentangan dengan update ini.

**Update RET-02 kuota/evaluasi:** mixed `get_context` maksimal dua handoff yang berhasil dimasukkan, dengan `truncated=true` bila kandidat layak dilewati karena kuota; `kind=handoff` eksplisit tidak terkena kuota tersebut (batas 12/24000 byte tetap). Guidance menjelaskan ranking/kuota dan tidak mengklaim deteksi testing otomatis. Evaluasi sintetis melalui transport MCP aktual menguji query Indonesia dan identifier kode, overview, keputusan, global berizin, pengecualian private/superseded, batas output serta override handoff. Go test/race/vet/build dan smoke sintetis lulus. Ini bukan penutupan RET-02: label testing terstruktur, dataset representatif pengguna, relevansi per-item dan RET-01 masih terbuka.

**Update histori/skill:** dashboard kini memuat histori 20 revisi per halaman melalui endpoint baru dan `?history=paged`; store memvalidasi seluruh rantai tetapi hanya menahan halaman+revisi sebelumnya. Legacy full history tetap kompatibel. Skill dapat menyalin judul/isi/sumber revisi lama ke revisi baru draft, dengan expected_revision, konfirmasi dan publikasi terpisah; relasi terbaru dipertahankan. Go test/race/vet/build, 30 frontend tests, Svelte build tanpa warning dan smoke sintetis lulus. Acceptance browser, diff lintas halaman, metadata review/skill dan keseluruhan REL-06/SKILL-01 masih terbuka. Pernyataan pagination histori belum ada pada checkpoint lama di bawah adalah historis.

**Update pagination key:** dashboard kini memakai `/api/keys-page`, default 50 key, maksimum API 100, cursor keyset dan audit metadata untuk halaman yang dimuat. Tombol sebelumnya/berikutnya tersedia; refresh/mutasi kembali ke halaman awal. Endpoint legacy dipertahankan. Go test/race/vet/build, 30 frontend tests, Svelte build tanpa warning dan smoke sintetis lulus. Ini mengurangi alokasi daftar pada jalur dashboard, bukan menyelesaikan pagination history, biaya SQL maksimum atau seluruh REL-06. Acceptance browser/rotasi lintas halaman tetap pending; pengganti harus terlihat pada halaman saat ini untuk melanjutkan revoke berpandu.

**Update eksekusi terbaru:** ranking kini tersambung ke MCP search/context dan pencarian dashboard. Kecocokan substring query utuh dipertahankan jika ada, fallback any-term leksikal jika tidak ada; title/content/source berbobot 6/2/1. Context tetap memprioritaskan overview dan mengecualikan superseded dari snapshot lengkap. Endpoint admin+CSRF `/api/duplicate-candidates` dan tombol opsional editor create/revise tersedia, maksimal 10 metadata kandidat satu proyek, tanpa merge/mutasi. Test scope/archive/partial identifier/invalid query/admin/CSRF lulus. Ditambah test writer lock SQLite nyata (40,28 ms untuk deadline 40 ms, bukan SLA) dan benchmark snapshot 100/1000 catatan. Go test/race/vet/build, 30 frontend tests, Svelte build tanpa warning, 45 Python tests dan smoke `.cache/tmp/smoke-k4rel7cj` lulus. RET-02/QUAL-01 maju tetapi acceptance browser, evaluasi dataset/kuota kategori dan integrasi kandidat untuk agen MCP belum lengkap. RET-01 indeks persisten, metadata review/skill, hook native dan operasi aktif belum dilakukan. Ringkasan lama berikut adalah checkpoint historis, bukan status terbaru.

Source paralel yang sempat terputus telah diperiksa dan disambungkan: editor create/revise admin+CSRF, diff teks, graph, filter lokal, registry API, context SQL dan mutasi pre-staging, serta limiter per-key. Go test/race/vet, build frontend tanpa warning, 21 test frontend, 45 test Python dan dua smoke binary sintetis lulus pada tahap ini; rincian di [verifikasi](verification.md). REL-06/UI/UX/SEC tetap **parsial**, bukan semua selesai. Registry belum mempunyai UI; helper ranking/duplikasi belum dihubungkan ke retrieval/UI; wizard/rotasi dan acceptance nyata belum selesai. Ekspor skill offline mempunyai test, bukan bukti discovery native. Lihat [ruang lingkup pengerjaan](backlog-execution.md).

## Tujuan dan cara menggunakan

Memona adalah pusat memori dan skill lintas harness berbasis Markdown. Fokus berikutnya: keandalan, kemudahan integrasi, kualitas retrieval, lalu distribusi skill dan otomatisasi.

- `[ ]` belum selesai. Gunakan `[x]` hanya setelah kriteria selesai terpenuhi dan ada bukti.
- Saat mulai task, tambahkan penanggung jawab/harness, status, dan dependensi yang relevan. Setelah selesai catat tanggal, file/commit bila tersedia, hasil test, serta batas verifikasi.
- ID task bersifat stabil untuk handoff dan pencarian memori. Jangan menomori ulang task lama.
- Prioritas P0: fondasi keandalan/keamanan; P1: penggunaan dan kualitas; P2: perluasan.
- Berkas ini menjadi daftar task lengkap. Catatan Memona adalah ringkasan/pengarah, bukan salinan kedua checklist yang diedit terpisah.
- Tidak ada penghapusan data, perubahan konfigurasi global, instalasi service, exposure, atau deployment otomatis. Task berisiko memerlukan persetujuan spesifik.

## Arah SaaS yang dipertimbangkan — belum keputusan migrasi

Pengguna mempertimbangkan Memona sebagai SaaS di masa depan. Diskusi disimpan di Memona `0049be8e1e7f9c1c9ca3c25c487aba4a` revisi 1 (22 Oktober 2026), bukan persetujuan implementasi SaaS. Rekomendasi awal: SQLite untuk lokal/self-hosted satu writer; PostgreSQL untuk SaaS bersama, dengan tenant terpisah dari project, pengguna/peran, isolasi semua akses, kuota serta kebijakan retensi/penghapusan. PostgreSQL untuk auth saja tidak mengatasi writer lock Markdown. Sumber utama konten SaaS (PostgreSQL atau object storage + metadata) dan dedicated instance vs shared tenant perlu keputusan tersendiri. Tidak ada perubahan stack, autentikasi, sumber utama Markdown atau deployment yang disetujui melalui diskusi ini; lanjutkan backlog saat ini.

## Baseline dan batas bukti

- Implementasi awal mencakup Markdown berversi, autentikasi admin/key, MCP, dashboard, arsip/restore admin, skill draft/publish, serta panduan agen.
- Test otomatis/build/smoke pernah lulus; lihat [verifikasi](verification.md). Tetap jalankan test relevan setelah perubahan.
- Pengguna melaporkan testing integrasi berhasil. Jangan menyamakannya dengan verifikasi lengkap seluruh kombinasi OpenCode/Hermes, backup/restore, browser, atau deployment publik.
- Periksa keadaan aktual sebelum mengerjakan task; backlog bukan pengganti pemeriksaan source, konfigurasi, dan data.

## Fondasi pengembangan

### DEV-01 — Baseline version control — P0

- [ ] Periksa apakah repository sudah tersedia; bila belum, usulkan inisialisasi dan baseline versi pertama.
- [ ] Verifikasi ignore terhadap data, key, konfigurasi runtime, cache, binary, dan fixture.
- [ ] Buat commit/tag hanya setelah diminta atau disetujui secara eksplisit.

**Selesai bila:** source dapat ditelusuri tanpa secret/data pengguna masuk repository; catat baseline yang disetujui. Tidak ada push otomatis.

### DEV-02 — Matriks acceptance nyata — P0

- [ ] Rekam versi aplikasi, OpenCode, Hermes, URL nonsecret dan skenario uji.
- [ ] Jalankan simpan/baca lintas sesi dan lintas harness, dashboard, konflik, revoke, serta lifecycle skill.
- [ ] Uji browser nyata: login, clipboard, keyboard, layar kecil, error dan one-time token.

**Selesai bila:** hasil pass/fail/blocked dan bukti tersunting tercatat; jangan menggantikan pengujian harness dengan klien sintetis. Rujukan: [integrasi](integrations.md).

## Tahap A — Andalkan penyimpanan dan pemulihan

### REL-01 — Backup konsisten — P0

**Status:** selesai untuk tooling offline dan pengujian sintetis; penanggung jawab OpenCode. Operasi terhadap data pengguna belum dijalankan.

- [x] Sediakan prosedur/command backup yang mencakup Markdown, revisi, database autentikasi/audit, dan konfigurasi yang diperlukan.
- [x] Tentukan strategi konsistensi: berhenti normal dahulu, atau snapshot terkoordinasi yang diuji. Backup SQLite saja tidak cukup.
- [x] Catat manifest versi, waktu, dan checksum; lindungi izin file backup.
- [x] Jangan menghapus backup lama otomatis sebelum kebijakan retensi disepakati.

**Bukti 22 Oktober 2026:** `scripts/recovery.py backup/verify`, lock writer eksklusif yang sudah ada, snapshot baru privat dengan manifest/checksum; unit test dan smoke sintetis lulus. Tidak mengambil backup data pengguna atau menghentikan layanannya. Lihat [operasi](operations.md) dan [verifikasi](verification.md).

**Selesai bila:** backup konsisten dapat dibuat dengan langkah singkat tanpa mencetak secret atau mengubah data aktif.

### REL-02 — Restore teruji — P0 — bergantung REL-01

**Status:** selesai untuk latihan sintetis terisolasi; penanggung jawab OpenCode. Cutover/data pengguna tetap memerlukan persetujuan terpisah.

- [x] Restore ke direktori baru, bukan menimpa data aktif.
- [x] Verifikasi login, key/scope/revoke, catatan, histori, arsip/restore, skill, dan audit.
- [x] Dokumentasikan bahwa restore dapat mengaktifkan kembali key yang dicabut setelah waktu backup; rekonsiliasi sebelum cutover.

**Bukti 22 Oktober 2026:** `scripts/recovery.py restore` membuat config loopback dengan data_dir salinan; `make recovery-smoke` membuktikan login/key/scope/revoke, revisi/relasi/arsip/skill/audit dan risiko revoke setelah snapshot. Fixture final `.cache/tmp/recovery-smoke-4gnvkf2a`. `make check` final lulus termasuk 19 recovery unit tests. Latihan sintetis lulus; pemulihan data pengguna, backup eksternal dan cutover belum dilakukan.

**Selesai bila:** latihan restore sintetis berhasil dan pemulihan/rollback terdokumentasi. Cutover produksi tetap memerlukan persetujuan.

### REL-03 — Idempotensi operasi tulis — P0

**Status:** implementasi dan test otomatis selesai, penanggung jawab OpenCode; aktivasi/reconnect serta acceptance harness nyata pending. Cakupan empat mutation tool MCP, bukan API dashboard admin.

- [x] Tambahkan identitas operasi/request yang dapat digunakan ulang saat retry.
- [x] Retry dengan identitas dan payload sama mengembalikan hasil yang sama; payload berbeda dengan identitas sama ditolak.
- [x] Batasi identitas operasi berdasarkan principal dan operasi; jangan membocorkan data lintas key/proyek.
- [x] Uji timeout setelah commit, restart, concurrency, dan save_handoff.

**Bukti 22 Oktober 2026:** `internal/memory/operations.go` menyimpan receipt hash bersama revisi canonical atomik, indeks turunan dibangun ulang saat startup. Field MCP `operation_id` opsional, retensi sepanjang histori tanpa TTL. Go tests/race/vet, Svelte check, 9 frontend dan 19 recovery tests lulus; binary smoke restart `.cache/tmp/smoke-hjrw9ag4` dan replay setelah backup/restore `.cache/tmp/recovery-smoke-o8nfyyj3` lulus. [Kontrak, retensi dan rollback](idempotency.md), [log/gap](verification.md). Tidak ada migrasi data aktif atau restart layanan pengguna; binary lama dapat menolak metadata `_operation` baru.

**Selesai bila:** retry operasi yang sama tidak membuat catatan/revisi ganda; perilaku retensi identitas dan pemulihan dijelaskan.

### REL-04 — Kesehatan penyimpanan — P0

**Status:** implementasi dan verifikasi otomatis lokal selesai; penanggung jawab OpenCode. Aktivasi layanan pengguna dan acceptance browser tetap pending. Tidak ada auto-repair/cleanup atau inspeksi data aktif pengguna.

- [x] Tampilkan kapasitas disk, kegagalan write, revisi rusak, dan staging tertinggal tanpa membocorkan isi.
- [x] Pisahkan health/liveness publik minimal dari informasi diagnostik admin.
- [x] Buat pemeriksaan integritas read-only, tidak menghapus atau memperbaiki file diam-diam.

**Bukti 22 Oktober 2026:** `internal/memory/health.go`, endpoint admin `/api/storage-health`, menu Svelte Penyimpanan dan CLI offline `inspect -data-dir`. Laporan aggregate tanpa filename/path/konten; counter kegagalan mutasi Markdown process-local, scan bounded/cancellable, existing shared lock menolak writer aktif. `make check` final lulus Go tests/race/vet, Svelte 0 error/0 warning, 11 frontend dan 19 recovery tests; build penuh serta binary smoke `.cache/tmp/smoke-2erdqcmh` dan recovery smoke `.cache/tmp/recovery-smoke-yyps0bcb` lulus. Hash/struktur/izin fixture tetap sama sebelum/sesudah inspeksi termasuk revisi rusak/staging. [Kontrak dan batas](storage-health.md), [log/gap](verification.md). SQLite integrity, probe kemampuan write, monitoring terus-menerus dan benchmark batas maksimum di luar cakupan.

**Selesai bila:** kerusakan/kapasitas rendah dapat didiagnosis dan dilaporkan tanpa merusak memori aktif.

### REL-05 — Pemulihan admin lokal — P0

**Status:** implementasi source/test sintetis disetujui; dua test kelayakan SQLite ditambahkan. Tertahan pada pengikatan direktori/sidecar: SQLite mengonversi URI descriptor menjadi path biasa. Belum ada CLI recovery atau perubahan autentikasi. Menunggu keputusan penanganan blocker, bukan persetujuan ulang scope yang sama. Rancangan bukan izin downtime/deployment.

- [x] Rancang reset password melalui akses OS lokal yang sah, tanpa endpoint reset publik bebas.
- [ ] Konfirmasi perubahan, jangan menaruh password di argumen proses/log; pertimbangkan lock dan proses aktif.
- [ ] Cabut sesi admin lama dan catat audit yang sesuai.

**Bukti rancangan 22 Oktober 2026:** [pemulihan admin lokal](admin-recovery-design.md) berdasarkan source access/CLI/lease yang ada. Usulan CLI offline dengan input terminal tersembunyi, lease exclusive existing, DB existing tanpa bootstrap/DDL, transaksi hash + expiry sesi + audit; API key/Markdown tidak berubah. Risiko pembukaan sidecar SQLite, pergantian target, terminal dan commit ambigu dirinci sebagai syarat implementasi/test, bukan dianggap telah teruji. Belum ada source autentikasi atau data aktif yang diubah; test fitur reset belum tersedia.

**Bukti kelayakan:** `internal/access/recovery_path_test.go` membuktikan `mode=rw` tidak membuat database hilang, tetapi URI `/proc/self/fd/...` tidak mempertahankan nama descriptor setelah canonicalization. Test Go/race/vet lulus; [bukti dan batas](verification.md#pemeriksaan-kelayakan-rel-05--22-oktober-2026). Berhenti sebelum kemampuan reset ditambahkan; kajian VFS/refactor atau perubahan kontrak eksplisit memerlukan keputusan tersendiri.

**Selesai bila:** password dapat dipulihkan tanpa menghapus database/memori, dengan test bahwa bootstrap biasa tidak menimpa admin lama. Eksekusi reset memerlukan persetujuan.

### REL-06 — Batas resource dan pembatalan — P1

**Status:** dikerjakan OpenCode, 22 Oktober 2026; increment baca menambahkan List/Get/HistoryContext dengan scan bounded/cancellable, SQL listing Keys/ActivityContext, bridge cancellation MCP–HTTP, satu snapshot search dan bounded backlink. Belum selesai: mutasi/validasi/startup/audit/SQL auth, cap output dan pengukuran maksimum masih pending. Tidak mengubah auth, dependency, data aktif atau layanan. Commit Markdown atomik tetap tidak dibatalkan di tengah. [Kontrak, pengukuran awal dan gap](resource-bounds.md); bukti aktual pada [verifikasi](verification.md).

- [ ] Teruskan context cancellation pada scan/SQL yang relevan.
- [ ] Batasi ukuran input/output, scan dan concurrency secara terukur; jangan membatalkan commit atomik di tengah.
- [ ] Uji request terputus dan beban bersamaan.

**Selesai bila:** pembatalan tidak menahan resource tanpa batas atau meninggalkan state setengah committed; batas terdokumentasi berdasarkan pengukuran.

## Tahap B — Permudah proyek dan integrasi

### UX-01 — Registry proyek — P1

**Update UI:** menu Proyek tersedia untuk daftar/cari/create/edit metadata, alias, arsip/pemulihan dengan expected_revision dan pesan konflik. Slug dari catatan yang belum terdaftar dapat didaftarkan tanpa migrasi; daftar slug catatan digabung dengan registry nonarsip untuk pilihan proyek. Alias tidak memperluas scope. Source, type-check, unit helper, backend tests dan binary smoke lulus; acceptance browser masih pending sehingga checklist keseluruhan belum ditutup.

- [ ] Kelola nama, slug stabil, deskripsi, alias/referensi lokasi, dan status arsip.
- [ ] Definisikan perilaku proyek belum terdaftar dan migrasi catatan/key lama secara kompatibel.
- [ ] Gunakan identitas yang sama untuk proyek pada mesin/worktree berbeda, tanpa memindai filesystem server otomatis.

**Selesai bila:** pemilihan proyek jelas, referensi lama tetap valid, dan alias tidak memperluas scope key.

### UX-02 — Wizard integrasi harness — P1 — bergantung UX-01

**Update UI:** panduan bertahap di MCP Server memilih OpenCode V2/Hermes, mencatat versi/profil di memori halaman, membuka form scope/izin/expiry, menghasilkan snippet tanpa secret, serta menjelaskan environment/reconnect/discovery/uji izin. Bentuk snippet diperiksa dari dokumentasi resmi saat implementasi. Tidak ada auto-install, uji koneksi dari browser memakai token, atau klaim kompatibilitas versi terpasang; acceptance dua harness pending.

- [ ] Pilih OpenCode/Hermes, versi/profil, proyek, izin, dan masa berlaku key.
- [ ] Hasilkan snippet konfigurasi yang benar, langkah reconnect serta uji read/write.
- [ ] Secret tampil sekali; jangan menampilkan ulang atau menyisipkan ke URL/log.
- [ ] Jelaskan runtime/service environment dan bedakan path lokal klien dari path server.

**Selesai bila:** pengguna dapat menghubungkan masing-masing harness mengikuti wizard tanpa mengedit bagian konfigurasi yang tidak relevan.

### UX-03 — Pengelolaan key lanjutan — P1

**Update audit:** dashboard kini menampilkan identitas `key:<id>` dan waktu/aksi operasi terakhir yang ditemukan dalam 200 event audit terbaru melalui endpoint admin `/api/key-usage`. Ini jendela histori terbatas, bukan telemetry koneksi/perangkat atau last-use global yang persisten; tidak ditemukan tidak berarti belum pernah dipakai. Go test/race/vet, frontend build/tests dan binary smoke lulus; acceptance browser/harness tetap terbuka. Pernyataan “belum direkam” pada checkpoint sebelumnya mengacu pada belum adanya telemetry khusus; audit yang sudah ada kini dipakai tanpa mengubah schema/auth.

**Update UI:** buat pengganti mempertahankan key lama; panel rotasi mengaitkan ID pengganti, mensyaratkan konfirmasi operator bahwa pengganti diuji, lalu membuka konfirmasi revoke lama. Pengganti harus ada/nonrevoked/nonexpired pada daftar terakhir. State rotasi hanya di memori halaman, bukan recovery workflow persisten. Penggunaan terakhir/identitas koneksi belum direkam dan UI menyebutkannya eksplisit. Revoke langsung tetap tersedia sebagai tindakan terpisah dengan konfirmasi dampak.

- [ ] Tampilkan penggunaan terakhir dan identitas koneksi, tanpa mengklaim online hanya dari histori.
- [ ] Sediakan alur rotasi: buat pengganti, uji, lalu revoke lama dengan konfirmasi.
- [ ] Jelaskan izin/proyek dan dampak revoke sebelum tindakan.

**Selesai bila:** rotasi dapat dilakukan tanpa membocorkan secret atau memutus koneksi lain yang tidak dipilih; test kedaluwarsa/revoke tetap lulus.

## Tahap C — Pertajam pencarian dan kualitas memori

### RET-01 — Indeks pencarian turunan — P1

- [x] Verifikasi dukungan SQLite FTS5 pada driver/build yang digunakan sebelum memilihnya (probe sintetis, bukan pilihan/implementasi indeks).

**Blocker rancangan terbaru:** indeks cache yang menghindari validasi histori setiap search mengubah kontrak deteksi korupsi di luar aplikasi. SQLite terpisah juga menghadapi pinning database/sidecar. Lihat [kajian indeks](retrieval-index-design.md); perlu keputusan kontrak sebelum implementasi, tidak memakai database autentikasi atau menurunkan validasi diam-diam.
- [ ] Indeks versi terbaru dan metadata; Markdown tetap sumber kebenaran dan indeks bisa dibangun ulang.
- [ ] Jaga sinkronisasi setelah save/revise/archive/restore/publish serta setelah crash.
- [ ] Filter akses sebelum mengembalikan hasil, termasuk cuplikan dan jumlah hasil.

**Selesai bila:** hasil konsisten dengan Markdown dan tidak bocor lintas scope; benchmark dibanding scan lama tercatat. Indeks tidak menggantikan backup autentikasi/audit.

### RET-02 — Pemilihan konteks relevan — P1 — bergantung RET-01

**Status terbaru:** helper ranking tersambung ke search/context MCP dan dashboard, dengan kompatibilitas substring-first dan fallback token. Snapshot izin tetap sebelum ranking; overview selalu dipertimbangkan dalam context meskipun query tidak cocok dan supersession diperiksa dari snapshot lengkap. Mixed context maksimal dua handoff; explicit kind=handoff mengikuti limit normal. Dataset sintetis end-to-end MCP untuk Indonesia/code/global/scope/supersession/kuota lulus. Penanda testing eksplisit dan penjelasan per-item sudah tersedia; batas JSON lengkap diuji. Dataset representatif nyata, kuota kategori lebih lengkap dan indeks RET-01 belum selesai.

- [ ] Prioritaskan ringkasan proyek, keputusan penting, handoff terbaru, dan temuan terkait tugas.
- [ ] Pisahkan catatan testing; batasi kuota per kategori agar catatan baru tidak menyingkirkan semua keputusan penting.
- [x] Tampilkan batas/truncation, sumber dan penjelasan relevansi yang bisa diperiksa (MCP; bukan confidence semantik).
- [x] Uji query Bahasa Indonesia, istilah kode, dan konteks lintas proyek yang diizinkan (sintetis; dataset pengguna belum tersedia).

**Selesai bila:** dataset uji menunjukkan konteks penting terambil dalam anggaran output; bukan sekadar mengambil terbaru.

### QUAL-01 — Deteksi duplikasi — P1

**Status terbaru:** endpoint read-only admin+CSRF dan pemeriksaan opsional pada editor create/revise sudah terhubung. Metadata kandidat maksimum 10, proyek sama, arsip dan ID editor dikecualikan; stale result disembunyikan setelah teks berubah. Skor leksikal bukan probabilitas duplikat. Tool MCP khusus `find_duplicate_candidates` sudah tersedia dan diuji scope/read-only/schema; acceptance browser tetap pending. Tidak memblokir save atau merge/hapus otomatis.

- [ ] Berikan kandidat catatan serupa sebelum/ketika menyimpan.
- [ ] Biarkan agen/pengguna memilih revisi atau catatan baru; tidak merge/hapus otomatis.

**Selesai bila:** saran membantu tanpa memblokir catatan berbeda yang sah dan tidak membocorkan proyek lain. Berbeda dari idempotensi REL-03.

### QUAL-02 — Konflik, penggantian dan informasi usang — P1

- [x] Tambahkan relasi supersedes eksplisit; lihat MEM-01 untuk implementasi dan batas.
- [ ] Tambahkan waktu verifikasi/perlu review (belum termasuk MEM-01).
- [ ] Bedakan keputusan pengguna, bukti terverifikasi, dan kesimpulan sementara.
- [ ] Tampilkan konflik untuk ditinjau; jangan menerapkan last-write-wins untuk kebenaran informasi.

**Selesai bila:** retrieval menyertakan status yang jelas dan informasi yang digantikan tidak dianggap keputusan aktif tanpa penjelasan; tidak menghapus histori.

### RET-03 — Evaluasi semantic search — P2 — setelah RET-01/RET-02

- [ ] Ukur kekurangan pencarian teks dengan contoh nyata sebelum menambah embedding.
- [ ] Tinjau biaya, privasi, model, resource, dan strategi rebuild.

**Selesai bila:** ada keputusan berbukti perlu/tidak perlu, bukan kewajiban memasang vector database. Pengiriman data ke layanan embedding memerlukan persetujuan terpisah.

## Tahap D — Skill bersama dan otomatisasi

### SKILL-01 — Metadata, review dan pengujian skill — P1

**Update:** diff tersedia untuk revisi pada halaman yang dimuat; admin dapat menyalin isi/judul/sumber versi lama menjadi revisi baru draft tanpa menimpa histori atau metadata relasi saat ini. Publish tetap terpisah. Metadata pemicu/versi/kompatibilitas/bukti test sudah tersedia pada store/MCP/editor/detail/exporter, disertai diff metadata. Tahap review formal/identitas reviewer terpisah belum ditambahkan, sehingga task keseluruhan belum selesai. Publish masih konfirmasi admin dan edit active menjadi draft; bukan verifikasi otomatis.

- [x] Tambahkan pemicu penggunaan, versi, kompatibilitas harness, sumber, serta hasil test (deklarasi penulis, opsional).
- [ ] Pertahankan draft → review → active dan review ulang setelah perubahan.
- [ ] Sediakan diff serta rollback/publikasi versi terdahulu sebagai perubahan baru, bukan edit histori.

**Selesai bila:** status publikasi dan bukti test dapat diperiksa; publish tidak dianggap bukti skill bebas risiko.

### SKILL-02 — Paket dan distribusi native — P2 — bergantung SKILL-01

- [x] Definisikan paket SKILL.md/references/assets dengan path aman, batas ukuran, dan versi (exporter offline reviewed, tanpa scripts; [kontrak](skill-bundles.md)).
- [ ] Verifikasi mekanisme discovery/auth OpenCode dan Hermes terpasang; jangan menganggap MCP otomatis mendaftarkan skill native.
- [ ] Ekspor atau sinkronkan hanya versi yang disetujui; jangan menimpa skill lokal tanpa persetujuan.
- [ ] Review skrip dan jangan mengeksekusinya otomatis karena publish/install.

**Selesai bila:** paket skill uji dapat dipakai kedua harness dan diperbarui dengan review/rollback yang jelas.

### AUTO-01 — Hook pengambilan konteks/checkpoint — P2 — bergantung REL-03

- [ ] Verifikasi hook/plugin pada versi tiap harness sebelum implementasi.
- [ ] Ambil konteks pada awal tugas serta checkpoint sesudah tahap penting dan saat perpindahan.
- [ ] Batasi biaya/token/frekuensi; laporkan kegagalan dan hindari duplikasi retry.
- [ ] Simpan ringkasan terstruktur, bukan seluruh percakapan secara otomatis.

**Selesai bila:** sesi baru dapat melanjutkan pekerjaan teruji, kegagalan save terlihat, serta tidak mengimpor secret atau percakapan mentah diam-diam.

## Jalur lintas tahap — keamanan dan operasional

### SEC-01 — Persiapan akses publik HTTPS — P0 sebelum exposure publik

- [ ] Verifikasi reverse proxy, hostname, sertifikat, Host/Origin, Secure cookie dan bearer MCP.
- [ ] Nonaktifkan allow_private_http pada konfigurasi produksi HTTPS; batasi akses backend.
- [ ] Uji Cloudflare Access untuk browser dan kebijakan akses mesin /mcp agar tidak mendapat halaman login HTML.

**Selesai bila:** uji positif/negatif berhasil pada konfigurasi yang disetujui. Tunnel/DNS/firewall tetap memerlukan izin; tidak ada port forwarding HTTP langsung otomatis.

### SEC-02 — Rate limit dan audit autentikasi — P0 sebelum exposure publik

- [ ] Tambahkan batas per-key serta pemisahan anggaran request valid/invalid, tetap dengan proteksi global.
- [ ] Jangan mempercayai header forwarded IP tanpa proxy tepercaya.
- [ ] Audit kegagalan login/key tanpa token/password/isi sensitif; tentukan retensi dan pertumbuhan audit.

**Selesai bila:** satu klien tidak mudah menghabiskan seluruh anggaran klien sah, revoke/expiry tetap langsung berlaku, dan log bebas secret.

### OPS-01 — Akun layanan dan startup otomatis — P0 sebelum operasi rutin

- [ ] Rancang eksekusi non-root dengan akses minimal pada direktori data.
- [ ] Siapkan template service, working directory, environment/secret, graceful shutdown dan restart.
- [ ] Dokumentasikan izin, backup, rollback dan pemantauan setelah reboot.

**Selesai bila:** template diuji pada lingkungan terisolasi. Instalasi service/perubahan ownership/permission produksi memerlukan persetujuan terpisah.

## Dashboard lanjutan

### UI-01 — Editor dan diff revisi — P1

- [ ] Buat/edit catatan melalui UI dengan validasi yang sama seperti MCP dan expected_revision.
- [ ] Tampilkan perbandingan perubahan; jangan mengedit berkas revisi lama.
- [ ] Uji CSRF, conflict, data invalid, XSS dan akses admin.

**Selesai bila:** koreksi dapat dilakukan tanpa kehilangan histori dan konflik tidak ditimpa diam-diam.

### UI-02 — Filter, testing dan kesehatan — P1

- [ ] Filter arsip, status, harness/penulis, tanggal dan penanda testing.
- [ ] Tampilkan versi aplikasi, keadaan penyimpanan, status backup dan error yang aman untuk admin.
- [ ] Pastikan kosong/loading/error, keyboard, tampilan mobile dan kontras tetap baik.

**Selesai bila:** admin dapat membedakan testing dari pengetahuan penting dan menemukan masalah tanpa membaca database/log mentah.

### UI-03 — Graph view memori — P1 — bergantung MEM-01

**Permintaan pengguna, 22 Oktober 2026:** tambahkan tampilan graph untuk menjelajahi memori yang sudah tersimpan. **Update:** source graph/API sudah diimplementasikan; iterasi terbaru mengikuti pola Obsidian: kanvas polos, titik abu-abu kecil, hover, label berbasis zoom, drag node, simulasi gaya berjeda/reduced-motion dan zoom pada kursor. Inspector/pengaturan tidak lagi permanen. Maksimal 100 node visual, daftar teks untuk hasil yang dimuat dan pemberitahuan batas. 30 test frontend, build Svelte tanpa warning, Go test/vet/build dan smoke sintetis lulus; acceptance browser nyata dan kemiripan visual masih pending. Tidak ada dependency baru, restart atau deployment. Rincian terbaru di [verifikasi](verification.md); checklist di bawah tetap memerlukan review acceptance, bukan klaim seluruhnya selesai.

- [ ] Tambahkan menu/tampilan Graph pada dashboard untuk menampilkan catatan sebagai node dan relasi eksplisit `related_to`/`supersedes` sebagai edge; bedakan jenis serta arah hubungan dengan legenda, bukan warna saja.
- [ ] Sertakan catatan yang belum memiliki relasi sebagai node mandiri; jangan menebak hubungan dari kesamaan teks, proyek, atau tanggal. Backlink merupakan arah balik relasi yang sama, bukan edge duplikat.
- [ ] Sediakan filter proyek, pencarian catatan, serta pilihan menampilkan arsip; tandai ringkasan proyek, jenis catatan dan catatan yang sudah digantikan.
- [ ] Klik/pilih node membuka ringkasan atau detail catatan; sediakan zoom, pan, reset tampilan dan fokus ke catatan beserta tetangganya.
- [ ] Ambil node/edge dari revisi Markdown terbaru dengan endpoint baca terautentikasi; pertahankan batas relasi satu proyek dan filter izin sebelum mengembalikan ID, judul, edge maupun jumlah hasil. Graph hanya membaca, tidak mengubah relasi melalui drag/drop.
- [ ] Tentukan batas node/edge dan perluasan bertahap agar koleksi besar tidak membekukan browser. Nyatakan hasil terpotong secara eksplisit; jangan menyajikan batas 20 referensi pada API relasi saat ini sebagai graph lengkap.
- [ ] Pertahankan gaya light/dark, responsivitas, reduced-motion, navigasi keyboard dan alternatif daftar hubungan yang dapat diakses tanpa mengoperasikan kanvas.
- [ ] Evaluasi kebutuhan library graph beserta lisensi, kompatibilitas Svelte/CSP dan biaya bundle sebelum penambahan dependency; tidak memerlukan graph database atau embedding hanya untuk visualisasi.
- [ ] Uji node mandiri, beberapa proyek, arsip, arah supersedes, pembaruan setelah revisi/arsip/restore, batas/truncation dan penolakan akses; lakukan acceptance browser untuk klik, keyboard, zoom/pan, mobile dan kedua tema.

**Selesai bila:** memori yang ada dapat dijelajahi melalui graph dan detail catatan dengan hubungan yang akurat, batas hasil jelas, tanpa membocorkan scope atau mengubah data. Hasil test otomatis dan acceptance browser dicatat terpisah.

## Urutan eksekusi yang disarankan

1. DEV-01/DEV-02 dan tahap A; dahulukan backup/restore serta idempotensi.
2. Tahap B untuk mengurangi kesulitan onboarding dan identitas proyek.
3. Tahap C agar retrieval tidak makin bising saat data tumbuh.
4. Tahap D setelah fondasi stabil; UI-01/UI-02 bisa dilakukan bertahap. UI-03 (graph view yang diminta pengguna) mengikuti MEM-01 dan diprioritaskan setelah backup/restore serta acceptance dasar.
5. SEC-01/SEC-02/OPS-01 wajib diselesaikan sesuai kebutuhan sebelum exposure/operasi rutin, tidak menunggu seluruh fitur lanjutan.

## Migrasi frontend Svelte — permintaan tambahan 22 Oktober 2026

### FE-01 — Source terpisah, distribusi satu binary

- [x] Svelte + TypeScript + Vite di `frontend/`, dependency terkunci dan cache lokal.
- [x] Migrasi login, daftar/filter, detail/riwayat, publish/archive/restore, key dan aktivitas tanpa mengganti API/data.
- [x] Build frontend sebelum Go; embed hanya dist dan sajikan aset dengan MIME/HEAD serta path terbatas.
- [x] Sidebar floating hide/unhide, ikon SVG lokal, indikator menu aktif beranimasi, reduced-motion.
- [x] Go test/race/vet, Svelte check dan unit test, binary smoke sintetis.
- [ ] Acceptance browser nyata: desktop/mobile, keyboard, fokus dialog/sidebar, clipboard, transisi dan CSP runtime.
- [ ] Restart/cutover aplikasi aktif setelah pengguna siap; bukan deployment otomatis.

**Status:** implementasi dan test otomatis selesai, acceptance browser serta aktivasi server pengguna pending. Bukti dan batas pada [verifikasi](verification.md); alur build pada [frontend](frontend.md). Tidak menandai UI-01/UI-02 atau task roadmap lain selesai hanya karena migrasi stack ini.

### FE-02 — Dashboard minimal glass dan MCP Server

- [x] Label MCP Server, card endpoint dari metadata admin dan card key di bawahnya.
- [x] Modal buat/detail/cabut akses, token sekali dengan konfirmasi sebelum tutup, alur key pengganti tanpa revoke otomatis.
- [x] Header/tagline/footer dekoratif dihilangkan; kontrol refresh/tema dan error tetap tersedia.
- [x] Tema light/dark melalui ikon, preferensi lokal tanpa penyimpanan secret.
- [x] Sidebar ciut menjadi rail ikon yang tetap interaktif, versi diperluas menampilkan label.
- [x] Build, Svelte check, 7 frontend tests, Go tests/race/vet dan smoke sintetis lulus.
- [ ] Acceptance browser: kedua tema, 375px/desktop, ikon rail, keyboard/fokus modal, clipboard, dan reduced-motion.
- [ ] Aktivasi binary pada server pengguna saat siap.

**Batas:** create/read/revoke key tetap kontrak yang tersedia; tidak ada edit izin atau hard delete baru. Lihat [frontend](frontend.md) dan [verifikasi](verification.md).

## Hubungan memori — permintaan tambahan 22 Oktober 2026

### MEM-01 — Ringkasan proyek → relasi → backlink

- [x] Ringkasan knowledge eksplisit, satu nonarsip per proyek, maks. 8000 byte; diprioritaskan get_context.
- [x] related_to/supersedes dalam metadata Markdown berversi, opsional agar klien/catatan lama tetap dibaca.
- [x] Validasi target satu proyek, ID unik/maks. 20, tanpa self-link atau siklus supersedes termasuk arsip, atomik terhadap writer bersamaan.
- [x] Backlink dari revisi terbaru sumber nonarsip, referensi satu-hop dibatasi dan difilter read scope.
- [x] Context mengecualikan catatan digantikan, search/read memberi peringatan; get_memory_relations tersedia.
- [x] Dashboard badge/urutan ringkasan, hubungan dapat dibuka, form revisi metadata dan histori relasi; isi tetap lewat MCP.
- [x] Build dan pengujian final lulus: Go tests/race/vet, Svelte 0 error/warning, 9 frontend tests dan smoke binary sintetis; bukti/batas di [verifikasi](verification.md).
- [ ] Acceptance browser dan dua harness nyata; belum mengganti acceptance RET-02/QUAL-02 keseluruhan.
- [ ] Backup konsisten, aktivasi binary/reconnect terjadwal dan pemilihan ringkasan/relasi nyata oleh pengguna.
- [ ] Evaluasi depends_on setelah penggunaan nyata; graph view telah diminta pengguna dan dirinci pada UI-03 (belum diimplementasikan).

**Batas:** [kontrak relasi dan rollback](memory-relations.md). Data aktif tidak dimigrasi/ditautkan otomatis, tidak restart. RET-01/RET-02 penuh, review timestamp, semantic search dan graph tetap backlog.

## Format catatan penyelesaian task

```markdown
### ID TASK — tanggal
- Status / penanggung jawab:
- Perubahan dan file/commit:
- Pengujian aktual dan hasil:
- Belum diverifikasi / batasan:
- Persetujuan atau keputusan yang masih diperlukan:
```

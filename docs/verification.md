# Laporan verifikasi pengembangan

## Startup lokal sederhana — 25 Oktober 2026

`make start` menjalankan binary yang sudah dibuild di foreground, tanpa `go run`/npm/build, tanpa membuat data atau config baru, dan menolak writer lain berdasarkan lock existing. `make status` hanya membaca lock dan melakukan probe `/healthz` ke loopback bila listener lokal/wildcard, tidak memanggil URL publik; kedua bukti diperlukan untuk klaim aktif pada data yang sama. Pada bind IP jaringan spesifik status tidak mem-probe jaringan dan hasil bisa belum terkonfirmasi. Bootstrap pertama, pengelolaan service dan recovery admin tetap terpisah.

Verifikasi aktual: `python3 -B -m unittest discover -s scripts -p 'test_*.py'` **76 lulus**, termasuk 4 test baru untuk preflight/data absen, lock sibuk, kombinasi lock/health dan Host pada wildcard loopback. `make status` pada konfigurasi proyek saat ini memberi hasil *tidak terkonfirmasi aktif* dan exit nonzero sebagaimana dirancang; tidak ada start, stop atau perubahan data. `git diff --check` lulus. Tidak ada test browser atau test Go yang diulang karena source Go/UI tidak berubah. Runtime start nyata belum dijalankan.

## Laporan acceptance pengguna — 25 Oktober 2026

Pengguna melaporkan bahwa poin uji browser dan alur OpenCode–dashboard–Hermes yang disarankan telah dicoba dan berhasil. Tidak ada versi/build/URL nonsecret, catatan pass/fail per langkah, hasil uji negatif, atau angka FPS yang diberikan. Ini bukti laporan pengguna, **bukan** acceptance yang dijalankan agen atau verifikasi bahwa semua kasus dalam `docs/integrations.md` telah lulus. Langkah backup konsisten dan latihan restore data aktif belum dilaksanakan; operasi tersebut memerlukan persetujuan spesifik serta downtime terjadwal. Catatan pending di bawah adalah riwayat pengujian agen, bukan penyangkalan atas laporan pengguna ini.

## Optimasi graph adaptif — 23 Oktober 2026

User menyetujui optimasi tanpa mengganti library. File baru `frontend/src/lib/graph-quality.ts` memusatkan profil kualitas, pemilihan label dan scheduler trailing timer+RAF. Renderer/wrapper/toolbar menggunakannya: mode Ringan saat >100node ATAU >300edge (DPR1, nodeResolution6, cooldown60tick/1800ms), normal DPR<=1.5/resolution12/cooldown120tick/3000ms. Seluruh500 node tetap diterima. LinkWidth0 memakai Three.Line, tanpa kurva kecuali self-loop, panah hanya relasi hover/selected. Default label hanya active; opt-in24/48 node terdekat, proyeksi/pembaruan dibatasi80/50ms, tidak memproyeksikan seluruh500 label per tick. Tidak ada physics warmup tambahan pada reduced-motion; seed layout deterministik tetap tersedia. Tidak mematikan render/picking ketika physics dingin: orbit/zoom/klik tetap tersedia; loop library masih aktif kecuali manual pause/visibility/dialog.

Review source independen menemukan dua masalah paused yang turut diperbaiki: data baru saat paused membutuhkan sinkronisasi mesh melalui satu tickFrame inner ThreeForceGraph dengan cooldown0 tanpa physics/resume; reset paused perlu mengatur target orbit ke pusat bounding box sebelum zoomToFit. Akses inner force object dibatasi duck-typed method pada scene versi dependency terpin, bukan perubahan source dependency; perlu acceptance saat upgrade.

Bukti aktual:

- `npm test`: **55 lulus**, termasuk adaptive threshold/dense links, label bounded500 input nonmutating, scheduler burst500 coalescing/trailing/dispose+latecallback dan source-contract renderer. Ini bukan browser test atau pengukuran FPS.
- `npm run build`: **0 Svelte error/0 warning**, Vite sukses; final `index-BClwiyEw.js`, `index-WIWl8UHS.css`, lazy `three-graph-renderer-iILbJV-V.js`. Warning chunk1393.63kB/373.57kB gzip tetap ada; optimasi ini runtime, bukan pengurangan download library.
- Offline `make test vet build-go GO=/usr/local/go/bin/go` lulus (server35.174s); setelah perbaikan paused final, build-go diulang lulus. Go source tidak diubah. Binary smoke final `.cache/tmp/smoke-jf4i5qd_` lulus; fixture sebelumnya `.cache/tmp/smoke-yn04yogm` juga lulus.
- Diff check ditinjau tanpa commit/hapus/stage. Tidak ada dependency baru, perubahan data/API/izin/config atau restart layanan pengguna.

Batas: belum ada browser/tools GPU untuk profil FPS/main-thread atau acceptance500 nodes. Perubahan mengurangi pekerjaan per frame secara struktural, bukan jaminan bebas lag setiap perangkat. Uji manual diperlukan: orbit/zoom/hover, label terdekat, paused->ubah filter/reset, reduced-motion dan buka/tutup note. Pengukuran FPS sebelum/sesudah belum tersedia; race tidak diulang untuk increment frontend-only.

## Revisi UI graph-first — 23 Oktober 2026

Permintaan pengguna: graph lebih besar dan fokus kanvas, filter rapi, kontrol ikon ringkas, hilangkan kontrol kamera tanpa gesture/penjelajah, klik node melihat isi. Skill UI/UX Pro Max digunakan: pencarian ikon pertama domain ux tidak tepat, dipersempit domain icons untuk accessible name/state; guidance Svelte derived/effect sesuai implementasi. Tidak memasang dependency baru.

Perubahan di `KnowledgeGraph.svelte`, `ThreeGraphCanvas.svelte`, `Icon.svelte`, App/style dan detail: heading ringkas, kanvas viewport besar, toolbar pencarian/proyek dan filter popover, kontrol ikon44px dengan glyph18px, legend kecil. Inspector/list permanen dan bagian kamera dihilangkan. Pencarian judul memberi maksimal20 tombol hasil metadata yang dimuat; Enter mencari server. Escape/outside/focusout menutup popover. Klik node langsung memakai endpoint admin detail yang sudah ada; dialog graph menampilkan isi goldmark safe-mode sebelum metadata, renderer dijeda selama detail/loading. Keyboard kanvas dan fallback select hanya ketika WebGL gagal dipertahankan. Responsive <=520px memakai search baris sendiri agar tidak terjepit sidebar pada375px. Data/API/izin/relasi tidak berubah, batas500 tetap.

Verifikasi aktual:

- `npm test`: **51 lulus**, termasuk4 regresi compile/source-contract baru untuk wiring buka detail/pause, dua branch konten eksklusif, kontrol ikon tanpa inspector dan fallback native. Ini bukan test DOM/browser.
- `npm run build`: Svelte **0 error/0 warning**, Vite lulus; aset final `index-CZe888jf.js`, `index-Bs-mDUoB.css`, lazy graph `three-graph-renderer-CPUcrZiF.js`. Warning chunk1393.02kB/373.37kB gzip tetap dilaporkan.
- Offline `make test vet build-go GO=/usr/local/go/bin/go` lulus (server34.437s); setelah penyesuaian CSS mobile final, build-go diulang lulus. Tidak ada perubahan Go source pada increment ini.
- Binary smoke final lulus, fixture `.cache/tmp/smoke-jgre2s5t`, termasuk embed entry/lazy chunk dan kontrak keamanan HTTP. Smoke terdahulu `.cache/tmp/smoke-1y_zovg7` juga lulus.
- Diff check lulus. Browser/tools/executable tidak tersedia, sehingga ukuran375px/GPU/gesture/fokus/visual light-dark belum diverifikasi manual. Race tidak diulang untuk increment UI-only ini.

Tidak ada restart/deployment, perubahan data contoh atau commit. Binary telah dibangun tetapi layanan aktif masih versi sebelumnya sampai aktivasi mendapat persetujuan. CSP upstream style-injection dan batas performa500 node tetap seperti catatan sebelumnya.

## Dataset contoh graph 500 — 23 Oktober 2026

Pengguna meminta 150 lalu tambahan 350 catatan sintetis. Proyek terpisah `demo-graph-500` kini memiliki tepat 500 catatan nonarsip (1 summary + 10 hub + 489 leaf), seluruhnya is_testing=true. Distribusi: knowledge191, decision120, handoff69, fact60, skill60; 440 unverified dan 60 skill draft, tanpa publikasi skill. Semua ditulis melalui MCP dengan operation_id; rate limit sempat menolak batch dan dipatuhi dengan pacing, bukan dinonaktifkan. Tidak ada penulisan file datastore langsung.

Verifikasi read-only terfilter proyek demo: 1476 related_to + 60 supersedes = 1536 relasi berarah, 1 komponen terhubung berisi 500 node, 0 node terisolasi, 0 referensi dangling, 0 siklus supersedes, 0 overlap kedua jenis relasi dan 0 judul duplikat. Data bukan fakta proyek atau hasil uji operasional. Tidak ada manifest dibuat; tidak ada penghapusan contoh otomatis.

Source visual diperluas ke batas500 (request API, layout, salinan data force engine, cache posisi, caption). Relaksasi awal deterministik dibatasi sekitar2 juta pair interactions dengan16 pass pada500 nodes, bukan180 pass untuk seluruh kapasitas. Test frontend47 lulus, Svelte0 error/0 warning, Vite/build-go offline lulus; warning lazy chunk besar tetap ada. Aset baru index-C5nEdL3l.js dan three-graph-renderer-CPUcrZiF.js. Browser/WebGL500 node belum diuji. Binary sudah dibangun tetapi belum diaktifkan untuk increment kapasitas ini; permintaan persetujuan restart ditutup tanpa jawaban sehingga tidak dilakukan. UI layanan lama masih membatasi100 node sampai aktivasi disetujui.

## Graph 3D — 23 Oktober 2026, pilihan akhir `3d-force-graph`

Pengguna menyetujui dependency lokal dan kemudian secara khusus memilih https://github.com/vasturiano/3d-force-graph. Renderer Three.js custom yang sempat dibuat diganti sebelum increment ini dilaporkan selesai. Implementasi akhir memakai `3d-force-graph@1.80.1`, `three@0.186.1` dan dev `@types/three@0.186.0`, versi dipatok lockfile; pemasangan lokal memakai `--ignore-scripts`, tidak ada instalasi global/deployment.

File utama: `frontend/src/lib/components/KnowledgeGraph.svelte`, `ThreeGraphCanvas.svelte`, `three-graph-renderer.ts`, `three-graph-layout.ts`, `force-graph-data.ts`, dua test pendamping, package/lockfile dan dist. Helper menyalin data sebelum d3 memutasi posisi/endpoint, membatasi 100 node, mempertahankan arah/jenis relasi serta node terisolasi/siklus. Renderer mempertahankan koordinat lokal/kamera lintas filter, drag node dan orbit, simulasi maksimal 180 tick / 5 detik, jeda/visibility/reduced-motion, label teks tanpa tooltip HTML, cleanup dan fallback context loss. Inspector di bawah kanvas menghindari menutupi klik; buka memori memakai tombol eksplisit, bukan double-click. Tidak mengubah API, izin, histori atau struktur memori menjadi pohon.

Bukti aktual:

- `npm test`: **44 lulus / 0 gagal**, termasuk determinisme layout/depth, bounds, cloning/endpoint mutation isolation, siklus/arah/jenis/dangling edge. Ini bukan test rendering GPU. Test fallback fake-DOM pada renderer custom tidak dipakai sebagai bukti implementasi akhir karena library memerlukan window/WebGL.
- `npm run build`: Svelte **0 error / 0 warning**, Vite sukses. Dist terbaru `index-DFpBuduC.js`, `index-C1xgUOrM.css`, lazy graph `three-graph-renderer-DM5CgE2I.js`. Chunk graph **1393.02 kB minified / 373.36 kB gzip**, Vite mengeluarkan warning >500 kB yang tidak disembunyikan; hanya dimuat ketika graph diperlukan.
- Offline `make test vet build-go GO=/usr/local/go/bin/go`: **lulus** setelah dist final, server 33.365s. Race tidak diulang khusus increment UI ini; hasil race SEC-02/QUAL-02 di bawah adalah bukti increment sebelumnya, bukan rendering baru.
- Python unit suite **72 lulus**. `scripts/smoke.py` final **lulus**, fixture `.cache/tmp/smoke-otg2_sy1`; tambahan smoke mengambil lazy chunk dari referensi entry JS dan memverifikasi aset disajikan binary embed. Tidak mengeksekusi WebGL.
- `npm ls --depth=0` menunjukkan dependency terpasang konsisten. Diff diperiksa tanpa menghapus dirty work sebelumnya; tidak ada commit/push atau aktivasi binary.

Keterbatasan: tidak ada browser executable/tool yang tersedia sehingga acceptance visual, GPU, pointer/touch/keyboard nyata, lifecycle dan context-loss belum diverifikasi. Library upstream menyisipkan style tag saat import yang dapat memunculkan CSP violation; CSP tetap `style-src 'self'`, tidak mengedit dependency/cache atau menambah unsafe-inline. Style wrapper lokal dan tooltip/nav bawaan dinonaktifkan, tetapi acceptance runtime CSP tetap perlu. Loop render library berjalan ketika tidak dijeda meskipun simulasi dingin; bukan renderer demand-only. [Checklist acceptance](frontend.md#acceptance-graph-3d-yang-masih-diperlukan).

## SEC-02 dan QUAL-02 — 23 Oktober 2026

Pengguna meminta rate-limit/audit keamanan serta waktu verifikasi/perlu review. Git tersedia; tujuh file sudah dirty sebelum pengerjaan dan dipertahankan. Pengguna melaporkan acceptance OpenCode; Hermes/harness lain tidak dianggap otomatis lulus. Tidak ada commit, restart/deployment, konfigurasi proxy/service/global atau perubahan data aktif.

Implementasi:

- `internal/server/security.go`, server wiring dan limiter: peer/preauth terpisah dari invalid/valid MCP, per-key sebelum shared valid, batas dua autentikasi di dalam delapan request global, login peer+aggregate, RemoteAddr tanpa forwarded trust. Kegagalan dicatat fixed actor/action tanpa token/password/alamat, maksimal satu percobaan/menit dan satu write in-flight 250ms, counter agregat admin `/api/request-security`. [Kontrak/retensi/gap](request-security.md).
- `internal/memory/review_metadata.go` + store/receipt: verified_at UTC kanonis self-declared, needs_review eksplisit non-skill, preserve/clear/status transition, immutable history dan receipt lama kompatibel bila metadata omitted. Tidak ada inferensi konflik atau waktu pemeriksaan otomatis. Server/UI/MCP/diff/filter meneruskan peringatan. [Kontrak dan rollback](review-metadata.md).

Verifikasi final aktual:

- Offline `make test vet build-go GO=/usr/local/go/bin/go` **lulus**; server test 32.705s.
- Offline `make race GO=/usr/local/go/bin/go` **lulus** setelah integrasi final; access 50.372s, memory 8.820s, server 133.225s. Run parent terdahulu timeout 300s saat pengujian paralel; tidak diklaim lulus. Run agen security sebelumnya juga lulus. Final run serial mempunyai timeout 600s.
- `npm test`: **40 lulus / 0 gagal**. `npm run build`: Svelte **0 error / 0 warning**, Vite sukses; dist `index-CDMSDh6_.js` dan `index-CfQasemn.css`. Ini helper/typecheck, bukan DOM/browser acceptance increment baru.
- Python unit suite: **72 lulus**. Smoke final `.cache/tmp/smoke-wblo4kgc` **lulus**, termasuk review metadata bertahan restart, endpoint counter/security dan revoke. Recovery smoke `.cache/tmp/recovery-smoke-gqp47psu` **lulus**.
- `git diff --check` lulus; diff source ditinjau, tanpa stage/commit/push atau mengubah dirty work sebelumnya.

Dua diagnosis integrasi dicatat, bukan disembunyikan:

1. Test empat concurrent idempotent retries sesekali mendapat 503 karena batas dua autentikasi baru. Test tetap memakai empat caller/real transport/production caps dan retry terbatas operation ID + payload yang sama pada 503; kesamaan revisi dan tidak adanya mutasi ganda tetap diperiksa. Security test terpisah mengunci batas concurrency.
2. Smoke membandingkan seluruh tree saat online-health gagal hanya pada `memona.db-shm`: pembacaan autentikasi sesi SQLite mengubah koordinasi reader SHM. Asersi online tetap membandingkan DB/WAL/Markdown/staging, kecuali bytes SHM volatile. Uji offline inspect tetap seluruh tree byte-identik; tidak menonaktifkan validasi durable storage.

Gap: audit append-only belum memiliki kebijakan retensi/monitoring disk; shared proxy/preauth/table/global concurrency masih dapat membatasi klien sah pada beban ekstrem; bukan fairness absolut atau DDoS protection. QUAL-02 tidak mendeteksi konflik semantik/expiry otomatis; waktu dapat dipertahankan saat isi berubah dan butuh re-review penulis. Binary dibangun tetapi belum diaktifkan. Metadata baru dapat membuat binary lama menolak histori; rollback membutuhkan pasangan data/binary kompatibel.

## Integrasi metadata, bundle, graph dan batas context — 23 Oktober 2026

Pengerjaan lokal OpenCode melanjutkan increment yang belum dilaporkan: metadata `is_testing` dan skill terstruktur dengan canonical/receipt compatibility, integrasi editor/MCP/exporter teks, 11-tool schema, MCP duplicate candidates, alasan pemilihan context, graph pointer/focus/RAF fixes, benchmark read serentak dan evaluasi semantic sintetis. Tidak ada aktivasi layanan atau migrasi data.

Batas context diperbaiki menghitung **seluruh JSON terstruktur** (notes, alasan, guidance dan metadata) maksimal 24000 byte; wrapper transport MCP/text duplikat tidak termasuk batas ini dan tetap tunduk cap HTTP. Test baru 12 note berisi 1750 byte menguji truncation dan keselarasan note/alasan. Suite schema awal sempat gagal HTTP 429 karena jumlah call bertambah; perbaikan suite memberi pacing 110ms antarpanggilan, bukan menonaktifkan limiter produksi. Suite penuh setelah perbaikan lulus.

Bundle offline `export_skill_bundle.py` yang tertinggal dari agen telah direview dan diselesaikan: manifest/note/supporting read dari descriptor staging sama, path/collision/size/content/mode checks, output baru dan manifest-last. Verifier terpisah memeriksa tree tepat/checksum secara read-only tanpa install/execute/repair. Test rename staging membuktikan note tidak dibaca ulang dari path pengganti. Satu test awal gagal karena fixture nested menciptakan intermediate directory 0755; helper fixture diperbaiki membuat setiap direktori baru 0700, tanpa melemahkan validasi. Ini bukan uji power loss, decoder raster atau jaminan melawan penyerang UID sama.

Bukti command aktual:

- `GOPROXY=off GOSUMDB=off make test race vet build-go GO=/usr/local/go/bin/go` **lulus**, semua package; race server 121.316s pada run gabungan terakhir. Tambahan Go setelah checkpoint: full-result budget test dan pacing schema. Tidak ada dependency download.
- `npm test` **38 lulus / 0 gagal** (helper/API/graph/editor/metadata diff, bukan DOM).
- `npm run build` **lulus**, Svelte **0 error / 0 warning**, Vite 138 module. Dist terbaru `index-DKoNbxvA.js` + `index-CfQasemn.css`.
- `make build-go` offline diulang setelah frontend final **lulus**.
- `python3 -B -m unittest discover -s scripts -p 'test_*.py'` **72 lulus**, mencakup 16 exporter bundle + 9 verifier bundle + 47 sebelumnya. Semua fixture sintetis ditahan dalam cache proyek; tidak cleanup.
- Binary smoke final **lulus**, fixture `.cache/tmp/smoke-bbnf7sjf`: assets embed, 11 tools, testing filter/opt-in, duplicate candidates, alasan context, idempotency/restart/revoke/health/inspect.
- Recovery smoke final **lulus**, fixture `.cache/tmp/recovery-smoke-_m1exlpo`: backup/verify/restore terisolasi, scope/revoke/notes/history/skill/audit dan replay, termasuk pelestarian is_testing/skill_metadata setelah restore. Tidak menyentuh data pengguna.
- Benchmark read dan semantic evaluation: lihat [hasil resource](resource-benchmark-2026-10.md), [evaluasi semantic](semantic-search-evaluation.md); full Go run juga mencakup test semantic final setelah metadata lengkap. Hasil 5/9 hanya fixture sintetis kecil, bukan dataset pengguna.
- Git status diperiksa: bukan repository; tidak ada klaim diff/commit. Source terkait dibaca ulang, gofmt dan typecheck dilakukan.

Gap tetap: browser nyata tidak tersedia, acceptance OpenCode–dashboard–Hermes belum dijalankan; review state/reviewer/verifikasi waktu belum diimplementasikan; RET-01 keputusan kontrak indexed read dan REL-05 pinning SQLite tertahan; limiter fairness/budget valid-invalid belum selesai; hook/discovery native belum diaktifkan. Binary dibangun tetapi tidak di-restart/deploy. Revisi baru dengan metadata nonempty tidak bisa dibaca binary lama tertentu; rollback butuh pasangan binary/data kompatibel. Catatan checkpoint bertanggal sebelumnya di bawah bersifat historis.

## RET-02 kuota handoff dan evaluasi MCP — 22 Oktober 2026

`internal/server/mcp.go` membatasi mixed context maksimal dua handoff yang benar-benar dimasukkan ke output; kandidat layak berikutnya dilewati dengan truncated=true. Permintaan `kind=handoff` eksplisit tidak terkena kuota ini, tetapi limit 12/anggaran 24000 byte tetap. Guidance mengungkap kuota, batas dan tidak adanya deteksi catatan testing otomatis. Tidak menambah metadata/protokol simpan, dependency, atau perubahan autentikasi.

`internal/server/context_evaluation_test.go` menyediakan corpus sintetis 14 catatan melalui transport MCP HTTP aktual: overview, keputusan, global yang diizinkan, private, keputusan superseded/pengganti dan delapan handoff. Dua query (`alur pasien`, `expected_revision`) dengan limit enam masing-masing mempertahankan empat ID relevan yang ditetapkan, dua handoff, overview teratas, tanpa private/superseded, dan berada dalam anggaran note JSON. Permintaan handoff eksplisit mengembalikan enam hasil dengan truncation. Ini precision/recall skenario kecil yang diharapkan, bukan benchmark dataset pengguna atau bukti semantic retrieval dan bukan harness OpenCode/Hermes nyata.

Verifikasi aktual: Go test/race/vet/build offline semua paket lulus; smoke binary sintetis `.cache/tmp/smoke-66b30rql` lulus. Source frontend/Python tidak berubah sehingga unit suite/build frontend dan unit Python tidak diulang. Browser/dua harness, dataset nyata dan performa maksimum belum diverifikasi. Binary dibangun, layanan tidak direstart, data aktif tidak diubah. Git diff tidak tersedia karena proyek bukan repository Git. RET-02 tetap parsial.

## Pagination histori dan pemulihan isi skill — 22 Oktober 2026

Store baru `internal/memory/history_page.go` memvalidasi seluruh rantai immutable dengan budget/context existing tetapi hanya menahan satu halaman dan revisi sebelumnya; newest-first, cursor `before` eksklusif, maksimum 50 item. Test mencakup urutan/cursor sampai akhir, input invalid, cancellation dan korupsi revisi lama di luar halaman. Endpoint admin `/api/notes/<id>/history-page` serta detail `?history=paged` di `server.go`/`history_page.go` diuji terintegrasi, termasuk boundary, default 20 dan kompatibilitas detail legacy yang tetap full/oldest-first. Tidak ada pengurangan validasi integritas atau klaim biaya scan menjadi konstan. Halaman full 50 revisi maksimum masih dapat melewati cap respons 8 MiB; dashboard memakai 20 dan error cap tetap fail-closed.

`skill_revision.go` menyediakan POST admin+CSRF `/api/notes/<id>/restore-skill-content`: salin judul/isi/sumber revisi tertentu ke revisi baru draft, expected_revision wajib, relasi terbaru dipertahankan melalui omission. Bukan rollback file, tidak mengedit histori dan tidak publish otomatis. Test baru memeriksa admin/CSRF, konflik stale, invalid source, non-skill dan histori tidak berubah. Jalur ini tidak idempotent; setelah disconnect/hasil ambigu periksa revisi terbaru sebelum retry. Audit best-effort mengikuti kontrak aplikasi existing.

App/NoteDetail/types memuat histori per 20 revisi, mengganti halaman (bukan append tanpa batas), navigasi lebih lama/terbaru dan aksi salin isi skill dengan konfirmasi. Diff terbatas pada revisi halaman yang dimuat; membandingkan lintas halaman belum tersedia. Posisi/fokus dialog dan aksi browser belum diuji secara manual. Tidak ada metadata skill baru atau schema autentikasi/Markdown yang diubah.

Bukti aktual: Go test/race/vet seluruh paket, frontend 30 tests, Svelte 0 error/0 warning + Vite build dan Go build offline lulus. Aset `index-rdsCztPT.js` / `index-D7M0bDBc.css`. Smoke binary sintetis `.cache/tmp/smoke-ogokryyh` lulus, bukan acceptance browser. Python unit suite tidak diulang karena source Python tidak berubah. Binary tidak diaktifkan/restart; data aktif/dependency/service tidak diubah. Tidak ada repository Git/diff/commit. REL-06 dan SKILL-01 tetap parsial, backlog keseluruhan belum selesai.

## Pagination key — 22 Oktober 2026

Ditambahkan `internal/access/key_page.go` / `key_page_test.go` dan `internal/server/key_page.go` / `key_page_test.go`. Endpoint admin GET `/api/keys-page` memakai keyset `created_at DESC,id ASC`, default 50/maksimum 100, membaca maksimum limit+1 key; cursor base64 JSON canonical divalidasi dan dipakai hanya sebagai parameter SQL. Tidak mengubah izin/autentikasi/schema atau endpoint `/api/keys` lama. Audit 200 event ikut diturunkan hanya untuk key halaman tersebut, tanpa memanggil listing key penuh. Test mencakup timestamp tie, urutan/halaman akhir, invalid cursor/limit, cancellation dan admin boundary.

`frontend/src/App.svelte` memuat endpoint baru, tombol sebelumnya/berikutnya, reset saat refresh/mutasi dan guard request/session existing. Pergantian halaman dinonaktifkan selama token sekali-tampil atau operasi simpan. Rotasi tetap konservatif: replacement harus ada pada halaman terakhir yang dimuat; tidak terlihat bukan berarti dicabut. Tidak ada snapshot lintas request atau total count; key baru di depan cursor terlihat saat refresh. Endpoint legacy tetap unbounded dan pagination ini bukan jaminan biaya scan/sort SQL karena tidak menambah indeks.

Bukti aktual: Go test/race/vet seluruh paket dan build-go offline lulus; frontend 30 test lulus, Svelte 0 error/0 warning dan Vite build (`index-BSwU-b6i.js`, `index-D7M0bDBc.css`) lulus; smoke sintetis `.cache/tmp/smoke-69wy3jb7` lulus. Browser pagination/rotasi nyata belum diverifikasi. Python unit suite tidak diulang pada tahap ini karena source Python tidak berubah. Binary tidak diaktifkan/restart; tidak ada data aktif, dependency atau service yang diubah. Proyek bukan Git repo, sehingga tidak ada diff/commit Git. Backlog keseluruhan belum selesai.

## Integrasi retrieval/duplikasi dan uji resource — 22 Oktober 2026

`internal/server/mcp.go` menghubungkan ranking helper ke search/context; `server.go` menghubungkannya ke dashboard query, mempertahankan arsip admin dan query partial identifier. Search mempertahankan himpunan substring utuh bila tersedia, baru fallback token; context memakai jalur yang sama lalu memprioritaskan summary. Supersession selalu dihitung dari snapshot lengkap, bukan hasil ranking. Dua kegagalan regresi OLD-MARKER pada search/context ditemukan dan diperbaiki dengan kontrak kompatibilitas tersebut, bukan melonggarkan test. Import strings tak terpakai pada percobaan awal sudah diperbaiki.

Endpoint baru POST `/api/duplicate-candidates` di `duplicates.go` memakai middleware admin/CSRF existing, validasi sebelum scan, metadata maksimum 10 kandidat proyek sama tanpa arsip/ID yang diedit. `duplicates_test.go` memverifikasi boundary, scope, redaksi konten, exclusion, tidak menambah revisi, input invalid, lexical fallback, ranking dan arsip/partial query dashboard. UI App/NoteEditor/NoteDetail/types menyediakan pemeriksaan manual opsional; snapshot teks menghindari menampilkan saran lama. Tidak ada perubahan protokol save, dependency, skema auth/Markdown atau merge otomatis. Tidak ada tool MCP duplikasi baru.

`internal/access/busy_context_test.go` memegang BEGIN IMMEDIATE dari koneksi SQLite terpisah. Audit dengan deadline 40 ms gagal pada 40,28 ms, tidak menghasilkan event, koneksi dapat dipakai kembali; test juga lulus dalam seluruh suite race. Ini bukti lokal driver/build saat ini, bukan jaminan hard deadline semua syscall.

Benchmark `retrieval_bench_test.go`, snapshot in-memory dengan sekitar 1 KiB konten/catatan, Go 1.26.8 Linux amd64 i3-10100T, 3 iterasi: ranking 100 catatan 2,07 ms/op, 1.450.397 B/op; duplikasi 1,64 ms/op, 1.255.010 B/op; ranking 1000 catatan 19,78 ms/op, 14.632.493 B/op; duplikasi 17,49 ms/op, 12.415.021 B/op. Setup dan scan disk tidak diukur, bukan peak RSS atau kapasitas maksimum/concurrency. Indeks persisten belum dibuat.

Verifikasi akhir aktual: Go test/race/vet/build offline seluruh paket lulus; 30 frontend tests lulus; build UI 0 error/0 warning, aset `index-25cKPWH0.js` / `index-D7M0bDBc.css`; 45 Python tests lulus; smoke binary `.cache/tmp/smoke-k4rel7cj` lulus. Acceptance browser/harness nyata belum dilakukan, layanan/data aktif tidak diubah, binary belum diaktifkan. Proyek bukan Git repo; tidak ada klaim diff/commit. Backlog keseluruhan belum selesai.

## Graph view bergaya Obsidian — iterasi interaksi 22 Oktober 2026

Revisi `KnowledgeGraph.svelte` mengutamakan kanvas polos, titik abu-abu kecil dan garis lurus tipis. Panel detail/pengaturan tidak mengambil kolom permanen; warna kategori opsional. Hover menyorot lingkungan, label mengikuti zoom/sorotan, node dapat ditarik dan double-click membuka catatan. Arah relasi tetap eksplisit di inspector/daftar teks, bukan panah pada kanvas. Helper baru `graph-physics.ts` menyediakan gaya tolak, pegas relasi dan zoom berjangkar kursor; test di `graph-physics.test.ts`. Simulasi memakai requestAnimationFrame, maksimal 600 langkah per restart, berhenti ketika tenang, dibatalkan saat unmount, bisa dijeda dan dinonaktifkan pada reduced-motion. Batas visual tetap 100 node; posisi hanya lokal dan tidak disimpan. Keyboard/pan buttons serta daftar teks tetap tersedia.

Bukti aktual: 30 frontend tests lulus, Svelte check 0 error/0 warning dan Vite build lulus (`index-D7M0bDBc.css`, `index-DGvaaiqK.js`). Go test/vet/build offline lulus dan smoke sintetis lulus (`.cache/tmp/smoke-d9nua5qn`). Error awal dua elemen style sudah diperbaiki; warning selector tak terpakai juga sudah diperbaiki. Tidak ada dependency baru atau perubahan backend. Race tidak diulang untuk tahap frontend ini. Kemiripan Obsidian, render browser, gesture/pointer capture, frame rate dan light/dark **belum diverifikasi di browser nyata**; test helper/smoke HTTP bukan bukti acceptance tersebut. Binary dibangun, tidak diaktifkan/restart.

## Penggantian Knowledge Graph — 22 Oktober 2026

App kini menggunakan `frontend/src/lib/components/KnowledgeGraph.svelte`, dengan helper `knowledge-graph.ts` dan dua test baru pada `frontend/tests/knowledge-graph.test.ts`. Peta jaringan menggunakan node kategoris, ukuran menurut derajat relasi, panah eksplisit, inspector dan fokus satu-hop, filter, pan/zoom serta fallback daftar teks. Layout sinkron dibatasi 100 node dan 180 iterasi; ini bukan bukti benchmark browser maupun graph lengkap. Tidak ada dependency baru, perubahan backend atau format data. Source graf lama tidak dihapus.

Bukti aktual tahap ini: `npm test` **28/28 lulus**; `npm run build` terakhir **0 error/0 warning**; aset `index-B8V-7dKT.css` dan `index-CcM8a5FU.js`. `GOPROXY=off GOSUMDB=off make test vet build-go GO=/usr/local/go/bin/go` lulus. `python3 -B scripts/smoke.py` lulus dengan fixture sintetis `.cache/tmp/smoke-yhswzncy`; smoke HTTP tidak memverifikasi render/interaksi graph. Race tidak diulang pada tahap UI ini. Browser visual, keyboard, pointer/touch, responsif dan light/dark tetap pending; binary tidak diaktifkan dan layanan tidak direstart. Proyek bukan repository Git sehingga tidak ada diff Git/commit.

Tanggal: 22 Oktober 2026. Lingkungan Linux amd64, Go 1.26.8. Pengujian memakai data sintetis, bukan memori pengguna. Bukan persetujuan deployment atau bukti production readiness.

## Lulus

| Pemeriksaan | Hasil |
| --- | --- |
| `make test` | Paket cmd, access, memory, dan server lulus; webui tidak mempunyai unit test terpisah |
| `make race` | Semua test lulus tanpa laporan race |
| `make vet build` | Vet lulus; executable dibuat di `bin/memona` |
| `node --check internal/webui/app.js` | Sintaks JavaScript valid |
| `python3 scripts/smoke.py` | Startup binary loopback, login, pembuatan key lewat API UI, MCP initialize/tools/save, pembacaan detail, restart/persistensi, dan revoke lulus |

Test otomatis mencakup hash key, pemisahan proyek/izin, revoke, bootstrap admin tidak ditimpa, sesi/logout, CSRF, Host/Origin, rendering Markdown tanpa HTML/URL skrip berbahaya, konflik revisi, penyimpanan immutable, symlink/traversal, writer lock, skill draft/publish, archive-only tanpa kebocoran isi, dan restore arsip admin. Lihat source test untuk kasus dan batasnya. Test memakai klien SDK atau HTTP sintetis, bukan proses OpenCode/Hermes sungguhan.

Pemeriksaan source independen menemukan pengembalian isi pada archive-only dan checkbox izin archive yang hilang; keduanya diperbaiki. Visibilitas publish juga dibatasi pada draft skill. Ini bukan audit keamanan formal atau pentest.

## Pembaruan akses jaringan

Listener nonloopback termasuk `0.0.0.0`, IP Tailscale, dan IPv6 kini diterima jika `public_url` HTTPS serta cookie Secure. HTTP jaringan biasa tetap ditolak. Test konfigurasi dan Host/Origin baru lulus; `make check build` serta smoke binary dijalankan ulang dan lulus setelah perubahan. Konfigurasi aktif, proxy, firewall, dan listener runtime tidak diubah. Lihat [network](network.md).

## Belum diverifikasi

Hasil khusus pemeriksaan kelayakan REL-05 tercatat di bagian akhir; belum ada kemampuan reset yang dapat diverifikasi end-to-end.

- Navigasi, layout responsif, clipboard, dan aksesibilitas di browser nyata; browser tidak tersedia pada lingkungan eksekusi ini.
- Lima langkah lintas sesi OpenCode dan Hermes dengan key produksi.
- HTTPS publik, Cloudflare Tunnel/Access, serta kompatibilitas header kebijakan deployment.
- Backup/restore operasional nyata, pemulihan kehilangan password admin, dan rotasi administratif produksi.
- Benchmark volume besar, indeks pencarian, test protokol resmi/conformance, audit supply chain atau pemindaian kerentanan lengkap.
- Skill native auto-discovery dan distribusi script/references. MVP menyimpan prosedur skill sebagai catatan dan memberikannya lewat MCP; template skill pengarah tersedia di `integrations/skills/memona/SKILL.md`, belum dipasang ke harness.

## Batas operasional yang masih ada

- Satu writer/proses untuk satu direktori data, filesystem lokal Linux; bukan cluster multiwriter.
- Pencarian memindai catatan beserta histori; output dibatasi, tetapi biaya scan tumbuh dengan data. Admission global dibatasi delapan request bersamaan; request lebih banyak mendapat 503. Pembatalan scan/SQL belum diteruskan menyeluruh.
- Limiter login/MCP global, bukan kuota per-key atau proteksi DDoS; gunakan proteksi edge tambahan sebelum publikasi.
- Transaksi Markdown dan audit SQLite bukan satu transaksi lintas media. Kegagalan audit dicatat di log, mutasi yang sudah committed tetap berhasil.
- API key bisa dibuat/dicabut dari UI. Mengubah izin dilakukan dengan membuat key pengganti lalu mencabut yang lama; bukan edit key lama.
- Tidak ada hard delete, reset password CLI, editor konten UI, secret scanner sempurna, embedding atau katalog native skill otomatis.
- Data bersifat plaintext pada disk. Lindungi akun OS, backup dan disk; konteks yang diambil bisa dikirim harness ke penyedia model.
- Sisa staging gagal tidak dibersihkan otomatis. Jangan menghapus catatan/revisi/lock untuk menyembunyikan error.

## Migrasi Svelte dan sidebar floating — 22 Oktober 2026

Implementasi dashboard diganti menjadi Svelte 5 + TypeScript + Vite, dibuild ke `internal/webui/dist`, di-embed ke binary Go. Fitur lama dipertahankan; filter jenis diselaraskan dengan backend. Sidebar floating memiliki hide/unhide, ikon SVG lokal dan indikator menu aktif beranimasi, dengan dukungan reduced-motion. Tidak memakai framework CSS/ikon tambahan.

Pengujian aktual setelah migrasi:

- `make build GO=/usr/local/go/bin/go`: npm ci tanpa lifecycle scripts, Svelte check **0 error/0 warning**, Vite build, lalu Go build lulus. Log lokal: `.cache/tmp/svelte-build.log`.
- `make check GO=/usr/local/go/bin/go`: Go test/race/vet lulus; Svelte check 0 error/0 warning; **5 frontend unit test** lulus (CSRF/cookie, stale response setelah logout, sesi berakhir, daftar nullable, validasi key). Log: `.cache/tmp/svelte-check.log`.
- `npm audit --omit=optional`: 0 kerentanan yang diketahui saat pemeriksaan; ini bukan audit supply chain lengkap.
- `python3 scripts/smoke.py`: lulus penyajian aset Svelte embed, CSP, HEAD tanpa body, penolakan path privat/legacy; login/key/MCP/save/detail/restart/revoke tetap lulus. Fixture sintetis: `.cache/tmp/smoke-chlju6yn`.
- Test Go baru mencakup aset build, MIME, HEAD, path privat/hilang, metode mutasi, serta isolasi API.

**Batas:** belum ada browser tersedia untuk menjalankan JavaScript dan memeriksa visual/klik/keyboard/animasi/clipboard secara nyata. Check compiler dan HTTP smoke tidak membuktikan acceptance browser. Dev proxy hot reload belum diuji dengan browser; ikuti konfigurasi loopback sintetis di [frontend](frontend.md). Tidak ada restart server pengguna, perubahan data aktif, konfigurasi produksi, atau pemasangan systemd.

## Dashboard glass, MCP Server, tema dan rail ikon — 22 Oktober 2026

- Modul Kunci API diganti label menjadi MCP Server. Card endpoint memakai GET `/api/server-info` yang memerlukan sesi admin dan hanya mengembalikan URL MCP konfigurasi serta transport; bukan secret, URL dari header pengguna, atau status koneksi palsu.
- Card pengelolaan key memakai modal native untuk pembuatan, detail, token sekali dan konfirmasi revoke. Membuat pengganti menyalin scope tanpa mencabut key asal. Tidak menambah operasi edit izin atau hard delete.
- Header besar/tagline/footer dekoratif dihilangkan. Status rutin tetap tersedia untuk screen reader; pesan error tetap terlihat. Tombol tema/refresh tetap dapat diakses.
- Sidebar floating saat diciutkan tetap menjadi rail ikon interaktif; ikon/label penuh kembali saat diperluas. Pada layar kecil default rail dan kembali ciut setelah pindah menu. Ikon memiliki accessible name/tooltip native, dan indikator aktif tetap beranimasi dengan reduced-motion.
- Glass light/dark memakai token warna; preferensi tema saja disimpan lokal, default mengikuti sistem. Pengujian hitung pasangan teks/button utama menghasilkan rasio 5.61:1 atau lebih; bukan pengukuran visual hasil komposisi kaca pada browser.

Verifikasi aktual: `make build GO=/usr/local/go/bin/go` dan `make check GO=/usr/local/go/bin/go` lulus; Svelte **0 error/0 warning**, **7 frontend unit test**, Go tests/race/vet termasuk pengujian autentikasi/read-only/URL metadata server. Log `.cache/tmp/glass-build.log` dan `.cache/tmp/glass-check.log`. Smoke binary lulus, termasuk pemeriksaan endpoint konfigurasi; fixture `.cache/tmp/smoke-aj3y7z8e`.

**Belum diverifikasi:** visual dark/light dan glass, klik rail/modal, focus return, clipboard, responsive, serta animasi pada browser nyata. Browser tetap tidak tersedia. Server aktif tidak direstart dan konfigurasi/data pengguna tidak diubah. Semua hasil di atas berasal dari source/compiler/unit/HTTP sintetis, bukan acceptance browser atau deployment production.

## Ringkasan proyek, relasi dan backlink — 22 Oktober 2026

Implementasi mengikuti urutan ringkasan proyek → related_to/supersedes → backlink. Metadata opsional disimpan pada revisi Markdown immutable; satu ringkasan knowledge nonarsip per proyek, maks. 8000 byte. Relasi maksimal 20 per jenis, satu proyek, tanpa self-link/duplikasi/siklus supersedes. Validasi dilakukan dalam mutex writer, termasuk keunikan ringkasan saat restore dan siklus melalui arsip.

get_context memprioritaskan ringkasan dan mengecualikan catatan digantikan nonarsip, dengan filter kind/limit dan anggaran objek JSON 24.000 byte. Search/read/skill memberikan referensi peringatan. Tool get_memory_relations menambah discovery menjadi sepuluh tool; backlink satu-hop diturunkan dari revisi terbaru dan difilter scope. Dashboard menyediakan badge/urutan ringkasan, navigasi referensi, serta revisi metadata dengan CSRF/expected_revision, bukan editor isi. Panduan MCP tertanam dan docs kontrak diperbarui.

Pengujian aktual:

- `make build GO=/usr/local/go/bin/go` lulus; Svelte **0 error/0 warning**, Vite dan Go build lulus. Log `.cache/tmp/relations-build.log`. Aset UI baru `index-BCoQcNxf.css` dan `index-D-yT7F1s.js`; aset lama tidak dihapus.
- `make check GO=/usr/local/go/bin/go` lulus, termasuk pengulangan final setelah tambahan test: `.cache/tmp/relations-check.log` dan `.cache/tmp/relations-check-final.log`. Go tests, race detector dan vet lulus; Svelte **0 error/0 warning**, **9 frontend unit tests** lulus.
- Test storage baru: lifecycle ringkasan/restore, batas ukuran saat metadata dihilangkan, preservasi/clear metadata, byte catatan lama tidak ditulis ulang, persistensi reopen/histori, target hilang/asing/invalid/ganda/self, siklus lintas arsip, konflik versi dan writer serentak.
- Test server baru: prioritas ringkasan/query/kind/limit, tidak memuat catatan digantikan, warning search/read, backlink, pembatasan scope/read/write dan redaksi respons write-only, skema MCP opsional, omit/clear lewat SDK, arsip/restore pengganti, anggaran output, batas 20 backlink, admin auth/CSRF/conflict serta metadata skill aktif kembali draft.
- `python3 scripts/smoke.py` lulus pada binary embed baru: login/key/MCP/discovery sepuluh tool, save ringkasan/supersedes, prioritas konteks, backlink, revisi metadata admin, restart/persistensi dan revoke. Fixture sintetis dipertahankan di `.cache/tmp/smoke-jzf9br9y`; bukan data aktif pengguna.
- Pemeriksaan link/fence docs lulus; file implementasi/test/docs yang diubah diperiksa whitespace. Proyek bukan Git repository, sehingga tidak ada klaim git diff/commit.

**Gap/risiko:** browser nyata dan acceptance dua harness belum dijalankan; backup/restore serta rollback operasional belum diuji. Retrieval masih scan Markdown dan perhitungan backlink per hasil, belum benchmark koleksi besar atau indeks. Tidak ada penetapan otomatis ringkasan/relasi pada data aktif, restart pengguna, konfigurasi production, dependency baru, atau penghapusan file. **Binary lama dapat menolak catatan yang sudah memakai metadata baru** karena canonical reader lama; lakukan backup konsisten sebelum aktivasi dan gunakan build/data yang kompatibel untuk pemulihan, jangan mengedit revisi lama. Lihat [kontrak relasi](memory-relations.md).

## REL-01/REL-02 — Tool backup/restore offline dan latihan sintetis — 22 Oktober 2026

Tool Python standard-library baru `scripts/recovery.py` menyediakan backup/verify/restore ke direktori baru, bukan subcommand binary. Backup memerlukan pengakuan stop normal dan lock writer eksklusif existing, mengambil seluruh data termasuk staging/WAL/SHM bila ada, konfigurasi asli, binary pilihan dan module manifest. SHA-256 per-file serta manifest versi/waktu, izin output 0700/0600, no-follow path, penolakan hardlink/device/FIFO, pemeriksaan perubahan sumber dan tidak ada cleanup. Restore memverifikasi inventory/hash sebelum membuat output, membuat config loopback ke salinan dan tidak mengeksekusi payload.

- `make check GO=/usr/local/go/bin/go` lulus: Go tests/race/vet, Svelte 0 error/0 warning, 9 frontend tests, **19 recovery unit tests** pada pengulangan final. Log `.cache/tmp/recovery-check-final.log` (pengujian sebelumnya dengan 16 test: `.cache/tmp/recovery-check.log`).
- `make recovery-smoke GO=/usr/local/go/bin/go` lulus memakai binary hasil build relasi yang sudah ada; tidak mengklaim rebuild binary baru karena source Go tidak berubah pada tahap ini. Fixture final `.cache/tmp/recovery-smoke-4gnvkf2a` (pengulangan sebelumnya `.cache/tmp/recovery-smoke-rggfr8ut`).
- Smoke menguji penolakan backup saat server aktif, stop normal server sintetis, backup tanpa perubahan sumber, restore bytes/config terisolasi, login admin lama, key/scope/revoke, histori/revisi, ringkasan/relasi, arsip/restore, skill aktif dan audit. Key yang dicabut setelah snapshot terbukti aktif lagi pada salinan restore; sebelum cutover perlu rekonsiliasi. Server uji dihentikan; fixture dipertahankan.
- Unit test mencakup overwrite refusal, checksum/tamper/missing/extra files, manifest traversal/type, symlink parent/hardlink/FIFO, private permissions tanpa chmod sumber, existing lock/missing lock, partial output retained, batas inventory/byte/depth dan command-line round trip/error tanpa isi secret sintetis. Python parse serta link/fence/whitespace dokumentasi diperiksa dan lulus.

**Batas:** ini pemulihan sintetis dengan binary nyata, bukan backup/pemulihan data pengguna, latihan disaster recovery lintas mesin, enkripsi/signature, maupun cutover production. Batas inventory 100.000 entri/16 GiB/16 MiB manifest/kedalaman 64 belum benchmark skala batas. Checksum bukan bukti keaslian tanpa digest tepercaya. Pelaku OS dengan akses tulis yang sama bukan threat model yang dapat dinetralisir oleh lock. Browser dan dua harness nyata tetap pending. Pembacaan konteks MCP pada awal tahap gagal karena output schema mismatch pada koneksi harness; konfigurasi harness tidak diubah dan issue reconnect belum diverifikasi.

### Diagnosis output schema MCP

- Ditambahkan `internal/server/mcp_schema_test.go`: `TestMCPDiscoveredOutputSchemas` melakukan discovery runtime pada server HTTP sintetis dan memvalidasi structured output **sepuluh tool** terhadap JSON Schema yang ditemukan. Termasuk hasil context/search kosong, ringkasan/relasi/backlink, skill aktif, save/revise/archive/handoff. Penambahan field tak dikenal sengaja ditolak sebagai kontrol validator strict. Memakai library jsonschema yang sudah ada di module graph; tidak mengubah dependensi atau source SDK.
- Test terarah `go test ./internal/server -run TestMCPDiscoveredOutputSchemas -count=1 -v` lulus. Setelah menambahkan test, `make test race vet GO=/usr/local/go/bin/go` lulus lagi; log `.cache/tmp/recovery-schema-go-check.log`. `make recovery-test` juga lulus kembali (19 test), log `.cache/tmp/recovery-schema-python-check.log`. `gofmt -d` test baru tidak menghasilkan diff.
- Tidak ditemukan pelanggaran output schema pada server lokal saat ini sehingga tidak ada perubahan kontrak server. Katalog harness sesi ini masih berbentuk delapan tool lama tanpa field relasi; stale discovery atau endpoint/build berbeda merupakan dugaan yang perlu diverifikasi setelah aktivasi/reconnect yang disetujui. Tidak mengklaim perbaikan koneksi live. Lihat [diagnosis integrasi](integrations.md).

## REL-03 — Idempotensi empat mutation tool MCP — 22 Oktober 2026

- `operation_id` opsional pada save_memory/save_handoff/revise_memory/archive_memory. Namespace API key ID/tool, fingerprint typed payload membedakan omit/clear, konflik payload berbeda; replay revisi commit awal tanpa mutation baru. Receipt internal hash disimpan dalam revisi Markdown yang sama, tanpa raw operation ID atau token, tanpa tabel SQLite baru. Indeks receipt turunan dibangun ulang saat startup; failed/ambiguous write menandai indeks dirty sebelum retry. Admin/legacy mutation tidak mewariskan receipt. Retensi mengikuti histori tanpa TTL.
- `make check GO=/usr/local/go/bin/go` final lulus: Go tests/race/vet, Svelte **0 error/0 warning**, **9 frontend tests**, **19 recovery tests**; log `.cache/tmp/idempotency-check-final.log`. `make build-go GO=/usr/local/go/bin/go` lulus, log `.cache/tmp/idempotency-build-final.log`; memakai aset UI yang sudah tersedia karena tidak ada perubahan frontend. Embedded agent guide telah diperbarui dalam binary final.
- Tambahan **12 top-level test storage** dan **4 top-level test server** memeriksa empat mutation tool, retry serentak/satu commit, payload berbeda serentak/satu konflik, namespace key/tool/proyek, original expected_revision/target, nil vs empty/false, replay setelah archive/restore/summary uniqueness/publish skill, fail validation tanpa reserve, corrupt/duplicate receipt, staging retained, closed process tanpa Close dan reboot store. Timeout setelah commit dimodelkan oleh HTTP transport yang membuang response sebelum memberi DeadlineExceeded ke klien; bukan klaim uji jaringan nyata atau putus daya.
- `TestMCPDiscoveredOutputSchemas` diperluas untuk operation_id opsional di runtime discovery serta output awal/replay yang tetap strict. Tes otorisasi memeriksa permissions/scope sebelum replay, revoke HTTP 401, write-only revise metadata redaction dan archive-only receipt tanpa isi. Pengujian awal menemukan principal ID pada access store berukuran 24 hex, bukan ID catatan 32 hex; validasi diperbaiki mengikuti kontrak access dan seluruh pengulangan final lulus.
- `python3 -B scripts/smoke.py` final lulus: discovery/retry/conflict/save_handoff/revise, original replay setelah restart, bersama regresi assets/auth/key/relations/revocation; log `.cache/tmp/idempotency-smoke-final.log`, fixture `.cache/tmp/smoke-hjrw9ag4`.
- `make recovery-smoke` final lulus: backup/restore mempertahankan receipt dan replay hasil awal walau catatan sudah direvisi; seluruh pemeriksaan recovery sebelumnya tetap lulus. Log `.cache/tmp/idempotency-recovery-smoke-final.log`, fixture `.cache/tmp/recovery-smoke-o8nfyyj3`. Fixture sintetis dipertahankan; server uji dihentikan.
- gofmt tidak menghasilkan diff setelah formatting. Tidak ada perubahan go.mod/go.sum/dependensi atau konfigurasi autentikasi. Source dan docs ditinjau lokal; tidak ada Git diff karena proyek belum repository Git.

**Batas:** belum diaktifkan pada layanan pengguna. `get_context` pada koneksi harness yang ada tetap gagal validasi additional properties; tidak ada reconnect/global config change. Acceptance browser dan dua harness nyata tetap pending. Receipt v1 baru dapat ditolak binary sebelum REL-03; backup pasangan binary/data kompatibel wajib sebelum aktivasi/rollback. Replay mempertahankan mutasi catatan, bukan exactly-once audit SQLite; event audit sukses bisa bertambah per replay. Retensi receipt dan indeks RAM tumbuh bersama histori tanpa quota/benchmark skala besar. Operasi setelah waktu snapshot tidak mempunyai bukti di restore; state klien perlu rekonsiliasi. Lihat [idempotensi](idempotency.md).

## REL-04 — Kesehatan penyimpanan — 22 Oktober 2026

- `internal/memory/health.go` menyediakan laporan kapasitas/inode, scan integritas Markdown, hitungan staging tingkat atas dan counter kegagalan mutation I/O/corrupt sejak Store mulai. Canonical/history validation dibagi dengan reader yang ada; receipt duplikat/summary/target/cycle latest diperiksa. API admin `GET /api/storage-health` hanya sesi cookie, no-store, tanpa path/nama file/isi/key; `/healthz` tetap liveness minimal, bukan readiness.
- Menu Svelte **Penyimpanan** dengan loading/error/incomplete, refresh manual, status berbasis teks, dark/light serta CSS responsif. Request guard/generation logout dan clearing existing diterapkan pada laporan; tidak ada polling/persistensi browser/auto-repair. CLI nyata `memona inspect -data-dir` membuka root/notes/staging/lock existing read-only tanpa writer Open/SQLite/chmod/create. Shared flock menolak writer aktif. Exit 0 bersih, 2 laporan warning/error/incomplete, 1 gagal memulai/output. Help flags tidak memantulkan path user melalui error mentah.
- `make check GO=/usr/local/go/bin/go` final **lulus**: Go tests/race/vet (termasuk test CLI baru), Svelte **0 error/0 warning**, **11 frontend tests**, **19 recovery tests**. Log `.cache/tmp/health-check-final-2.log`. Run awal `.cache/tmp/health-check.log` terhenti oleh timeout harness 120 detik saat race; pengulangan dengan 300 detik lulus, tidak ada test dinonaktifkan.
- Test baru `internal/memory/health_test.go`, `internal/server/health_test.go`, `cmd/memona/inspect_test.go` memeriksa immutable byte/mode/path snapshot online/offline, closed store, busy mutex/active writer, corrupt history yang membuat Open gagal, redaksi staging/contents, symlink/FIFO/hardlink/public/oversize revisions, nama tak dikenal, missing root/lock tanpa create, cancellation/entry/byte budgets, disk rendah/unavailable, duplicate receipt/summary/cycle/invalid target/history transition, process-local write failures versus validation/conflict, auth/session/logout/Host-Origin/CSRF dan liveness independen dari store. Limit tests memakai budget/Statfs sintetis, bukan memenuhi disk pengguna atau benchmark koleksi maksimum.
- `make build GO=/usr/local/go/bin/go` final lulus; log `.cache/tmp/health-build-final.log`. Embed terbaru: `index-RNSd3c_s.css` dan `index-DCtY_HG2.js`. Aset lama tidak dihapus otomatis. Tidak ada dependency/go.mod/go.sum/security configuration change.
- `python3 -B scripts/smoke.py` final lulus: regresi auth/MCP/retry/relations/restart/revoke, admin health tanpa perubahan byte, penolakan inspector saat writer aktif, injeksi revisi rusak/staging sintetis, liveness minimal, inspector offline exit 2 dan hash sumber tetap sama. Log `.cache/tmp/health-smoke-final-2.log`, fixture `.cache/tmp/smoke-2erdqcmh`. Assertion pertama keliru mengharapkan lima catatan, diperbaiki ke empat catatan unik; `.cache/tmp/health-smoke.log` menyimpan kegagalan awal, bukan bug validasi aplikasi.
- `make recovery-smoke` final lulus pada binary baru: backup/restore, snapshot revocation, history/relations/skill/audit dan operation replay tetap lulus. Log `.cache/tmp/health-recovery-smoke-final.log`, fixture `.cache/tmp/recovery-smoke-yyps0bcb`. Server sintetis dihentikan normal; fixture dipertahankan, tidak ada cleanup.
- Pembacaan live `get_context(project=memona, query=REL-04)` pada awal tahap ini berhasil, berbeda dari error schema yang dicatat sebelumnya. Tidak ada perubahan/reconnect konfigurasi harness oleh agen dan penyebab perubahan live tidak disimpulkan; ini bukan bukti acceptance lengkap OpenCode/Hermes atau aktivasi build REL-04.

**Batas:** [kontrak storage health](storage-health.md). Scan paling banyak 20.000 entri/256 MiB, timeout/cancellation kooperatif 5 detik tidak menginterupsi syscall disk macet; scan online menahan mutex writer sampai selesai. Read-only tidak menjanjikan atime filesystem tetap sama. Laporan tidak meliputi SQLite/auth/audit integrity/probe write/readiness/semantic truth/monitoring/quota. Counter write failure tidak persisten atau mencakup startup/Close/SQL; nol bukan bukti writeability. Ruang/inode/volume maksimum belum benchmark. UI keyboard/mobile/visual nyata belum diverifikasi; activation/downtime/data pengguna tetap perlu persetujuan. Tidak ada perubahan format revisi/data migration baru pada REL-04; risiko rollback REL-03 tetap berlaku.

## Lingkungan setelah REL-04

Tidak ada konfigurasi global harness, service permanen, tunnel, DNS, firewall, atau deployment publik yang diubah. Tidak membuat commit atau push. Folder ini belum repository Git; tidak ada diff terhadap baseline Git yang dapat dilaporkan. Binary, module/build caches dan fixture uji adalah hasil lokal yang dikecualikan `.gitignore`.

## REL-06 tahap baca — 22 Oktober 2026

Increment baca, **belum penyelesaian REL-06**. [Kontrak resource, guardrail dan gap](resource-bounds.md). Pertimbangan SaaS tersimpan dan dibaca ulang sebagai knowledge `0049be8e1e7f9c1c9ca3c25c487aba4a` revisi 1; bukan persetujuan migrasi PostgreSQL atau perubahan sumber utama Markdown.

- `internal/memory/read.go`: List/Get/HistoryContext, batch nama, shared budget per operasi 20.000 entri/256 MiB/5 detik, mutex wait cancellable tanpa goroutine pembantu, validasi seluruh histori dengan latest-only retention untuk Get/List. Helper pembacaan revisi di store.go berbagi validasi canonical lama; legacy startup/mutasi tidak diberi limit baru.
- `internal/access/store.go`: KeysContext/ActivityContext menggunakan QueryContext; wrapper lama dipertahankan. Tidak mengubah SQL auth/mutasi/skema.
- Server meneruskan context baca ke scan/filter/relasi dan SQL listing, menggabungkan cancellation HTTP/SDK MCP, search satu snapshot, backlink bounded 20 per kelompok saat pengumpulan, admin notes menghitung relasi setelah cap kandidat 500. Error cancel/deadline/budget aman tanpa hasil parsial.
- Test memory baru memeriksa kesamaan hasil dengan jalur lama, histori empat revisi, korupsi revisi awal tetap ditolak, context cancel/deadline ketika mutex ditahan, operasi berikut tetap berfungsi, exact/+1 budget entry/byte, closed/invalid input.
- Test access baru memeriksa daftar kosong/normal/order/limit activity 200, row persisted tidak berubah, cancel sebelum query dan ketika menunggu satu koneksi (sinkronisasi DBStats.WaitCount), serta koneksi tidak bocor.
- Test server baru memakai HTTP loopback nyata: 8 slot penuh, request ke-9 503 lintas endpoint, disconnect/error melepas slot. Tool sintetis dengan protokol legacy `2025-03-26` dibatalkan oleh disconnect HTTP tanpa notifikasi cancel; cancellation SDK tetap dipertahankan dengan request context independen. Test memeriksa deadline tool, fan-in 1.024/4.096, urutan deterministik/truncation/filter dan cancel termasuk final check tanpa partial output. Ini bukan uji mutasi ketika disconnect.
- `GOPROXY=off GOSUMDB=off make test race vet build-go GO=/usr/local/go/bin/go` **final lulus** setelah review dan tambahan assertion, log `.cache/tmp/resource-read-final-check-2.log`. Tidak ada laporan race. Run sebelumnya juga lulus, log `.cache/tmp/resource-read-test-vet.log`, `.cache/tmp/resource-read-race.log`, `.cache/tmp/resource-read-final-check.log`.
- Review source menemukan key write-only/archive-only dapat memicu snapshot sebelum read ditolak. Diperbaiki dengan pemeriksaan read sebelum ListContext; test nil-store membuktikan jalur denied tidak menyentuh storage. Test error/status JSON dan API listing dengan context sudah canceled juga lulus.
- `BenchmarkListContext` 3 iterasi/skenario lulus: 32 × 1 revisi ~1,48 ms/op, 395.442 B/op; 64 × 8 ~17,72 ms/op, 5.988.386 B/op. Konten 1 KiB, log `.cache/tmp/resource-read-bench.log`, fixture retained `.cache/tmp/read-bench-*`. Bukan peak RSS, SLA, benchmark batas maksimum atau concurrency scan.
- `make build-go` lulus, log `.cache/tmp/resource-read-build.log`; memakai dist frontend existing, tanpa install/build frontend. `python3 -B scripts/smoke.py` lulus pada binary baru: regresi auth/MCP/schema/relations/retry/restart/revoke/health/inspect. Run awal log `.cache/tmp/resource-read-smoke.log`, fixture `.cache/tmp/smoke-n7xviaga`; **smoke final setelah review fix** juga lulus, log `.cache/tmp/resource-read-smoke-final.log`, fixture `.cache/tmp/smoke-7dffqjjj`.
- `make recovery-test recovery-smoke` lulus: 19 Python tests dan binary backup/restore/replay; log `.cache/tmp/resource-read-recovery.log`, fixture `.cache/tmp/recovery-smoke-_k2d9of3`. Tidak ada cleanup sumber atau fixture; hanya proses sintetis yang dihentikan.

**Gap:** cancellation/budget mutasi, startup/rebuild receipts/validasi cycle, SQL auth/audit/mutasi, hard cap output/history pagination, SQLite busy wait dan benchmark maksimum/concurrency scan masih pending. Deadline scan bukan deadline seluruh handler karena auth/audit/PBKDF2/syscall/encoding tertentu belum cancellable. Read lebih besar dari guardrail bisa ditolak meskipun startup masih berhasil. Tidak mengubah data aktif/service/global config/dependency atau mengaktifkan binary pada layanan pengguna; acceptance browser/OpenCode/Hermes tetap pending. Proyek belum Git repository, tidak ada diff/commit/push yang diklaim.

## Pemeriksaan kelayakan REL-05 — 22 Oktober 2026

- Persetujuan source/test sintetis diterima. Belum menambahkan CLI recovery, pembaruan hash, expiry sesi, input terminal atau transaksi reset. Implementasi dihentikan pada blocker filesystem sesuai [desain](admin-recovery-design.md).
- Dua test di `internal/access/recovery_path_test.go` lulus: URI SQLite `mode=rw` menolak database hilang tanpa membuat file; URI `/proc/self/fd/<dirfd>/memona.db` justru dikonversi menjadi path biasa pada `PRAGMA database_list`. Setelah rename direktori fixture, descriptor tetap menunjuk database, sedangkan filename SQLite menunjuk lokasi lama. Ini membantah shortcut descriptor sebagai bukti pinning direktori/sidecar; tidak mengklaim uji eksploit race atau pembukaan WAL setelah rename.
- Probe awal gagal saat bootstrap memakai path sintetis berisi `?`; `access.Open` existing memakai DSN biasa. Fixture kemudian di-bootstrap pada path sederhana dan di-rename sebelum uji URI. Tidak mengubah test untuk mengeklaim bootstrap nama khusus sudah berfungsi dan tidak mengubah source startup. Log awal `.cache/tmp/admin-recovery-path-probe.log`; hasil dua test final `.cache/tmp/admin-recovery-path-probe-2.log`.
- `GOPROXY=off GOSUMDB=off make test GO=/usr/local/go/bin/go` lulus, log `.cache/tmp/admin-recovery-probe-tests.log`.
- `GOPROXY=off GOSUMDB=off make race GO=/usr/local/go/bin/go` lulus tanpa laporan race, log `.cache/tmp/admin-recovery-probe-race.log`.
- `GOPROXY=off GOSUMDB=off make vet GO=/usr/local/go/bin/go` lulus, log `.cache/tmp/admin-recovery-probe-vet.log`.
- Seluruh run memakai Go 1.26.8 dan cache proyek; fixture `.cache/tmp/admin-path-probe-*` dipertahankan. Tidak ada dependency/module cache/go.mod/go.sum diubah, pemasangan software, cleanup atau perubahan data aktif. Folder belum Git repository; `git status` gagal dengan pesan bukan repository, bukan diff bersih.

**Belum diuji:** terminal/PTY, atomisitas reset, sesi/key/audit sesudah reset, recovery binary smoke/crash dan filesystem recovery menyeluruh karena belum ada implementasinya. Build/frontend/browser/harness acceptance tidak dijalankan ulang pada tahap test-only ini. Menunggu keputusan kajian VFS/refactor, revisi kontrak eksplisit, atau penundaan REL-05; tidak ada reset maupun restart layanan yang diizinkan oleh hasil ini.
# Penggunaan key dari audit — 22 Oktober 2026

Ditambahkan endpoint admin read-only `/api/key-usage` di `internal/server/server.go`, derivasi `key_usage.go` dan test `key_usage_test.go`; App/Keys/types menampilkan identitas audit serta operasi terakhir tercatat. Tidak ada perubahan autentikasi, skema SQL, penyimpanan token, pengumpulan IP/User-Agent atau klaim online. Lookup mencocokkan Actor key tepat terhadap audit yang sudah ada, mengambil operasi memory/skill pertama dalam urutan newest-first dari maksimum 200 event. Tidak ada temuan **bukan berarti belum pernah digunakan**; key-only discovery/initialize dan audit yang gagal dicatat tidak terlihat. Nama key saat ini immutable; bila kontrak rename berubah, matching actor perlu ditinjau.

Bukti aktual: Go test/race/vet seluruh paket lulus; 26 frontend tests lulus; Svelte build 0 error/0 warning dan aset akhir `index-C6O7dhb2.js`/`index-mBzPlHKg.css`; build-go lulus; smoke binary sintetis lulus, fixture `.cache/tmp/smoke-4qxya_gc`. Test baru mencakup exact actor (bukan prefix), event terbaru, filter jenis operasi, admin-only serta tidak mengirim token/target note.

Browser tools tidak ditemukan melalui discovery tool sesi; paket Playwright/Puppeteer juga tidak tersedia dalam dependency frontend yang diperiksa. Karena itu acceptance DOM/keyboard/mobile/clipboard/theme **belum dilakukan**, tidak digantikan smoke HTTP dan tidak ada dependency diinstal. Harness pengguna tidak diakses, binary belum diaktifkan, layanan tidak direstart. Aktivasi, akses harness uji, atau penyediaan browser memerlukan langkah/persetujuan terpisah.

# UI proyek, integrasi dan rotasi — 22 Oktober 2026

Permintaan pengguna: kerjakan UI yang belum selesai. Ditambahkan `Projects.svelte`, `projects.ts`, `IntegrationGuide.svelte`, `integration-guide.ts`, dan dua file test helper; diperbarui App, Keys, types, Icon dan dist. Tidak ada perubahan backend/auth/dependency pada tahap UI ini.

- `npm run build`: lulus Svelte 0 error/0 warning dan Vite. Aset akhir `index-mBzPlHKg.css`, `index-VA3It7KL.js`.
- `npm test`: **26 lulus**, termasuk validasi registry byte/slug/alias/revisi, bentuk snippet/env terpisah, penolakan URL credential/control/query dan syarat rotasi.
- `GOPROXY=off GOSUMDB=off make test vet build-go GO=/usr/local/go/bin/go`: lulus semua paket, vet, binary embed baru. Race tidak diulang tahap ini karena tidak ada perubahan Go/runtime concurrency.
- `python3 -B scripts/smoke.py`: lulus dengan binary terbaru; fixture sintetis `.cache/tmp/smoke-yvzg9e2j`. Ini memverifikasi aset embed/CSP dan API, **bukan interaksi browser DOM**.
- Dokumentasi resmi MCP OpenCode V2 dan Hermes dibaca untuk memeriksa field snippet. Tidak ada instalasi/konfigurasi harness nyata yang diakses atau diubah.
- Git tetap bukan repository; tidak tersedia diff baseline. Tidak ada file dihapus, cleanup aset lama, dependency install, restart atau aktivasi binary.

Gap: browser nyata (keyboard, modal, mobile, clipboard, theme), acceptance dua harness dan penggunaan terakhir key. Panduan/rotasi merupakan UI operasional manual; bukan pengujian koneksi otomatis. Registry/UI yang baru tidak berarti seluruh backlog selesai.

# Integrasi backlog lanjutan — 22 Oktober 2026

Pemeriksaan source setelah pekerjaan paralel terputus menemukan helper `rebuildOperationsBudget` belum tersedia. Helper dilengkapi dengan budget dan publikasi indeks setelah scan lengkap; test canceled/limit rebuild dan replay ditambah. API editor/registry serta context mutasi MCP/dashboard disambungkan; konflik rune Svelte dari nama `state` diperbaiki. Editor create/revise kini terhubung ke App, graph tetap memakai endpoint admin, filter lokal ditandai hanya atas hasil dimuat.

Hasil aktual (test routing registry terakhir juga telah lulus lewat pengulangan `make test vet`; perubahan terakhir hanya test/dokumentasi, race/build sebelumnya mencakup source runtime yang sama):

- `GOPROXY=off GOSUMDB=off make test`: seluruh paket lulus.
- `make race vet build-go` dengan environment offline sama dan Go `/usr/local/go/bin/go`: lulus. Percobaan awal race terkena timeout tool 120 detik; pengulangan 300 detik selesai (server race sekitar 74,5 detik).
- `npm test`: 21 lulus; `npm run build`: Svelte 0 error/0 warning, Vite sukses tanpa instalasi dependency. Aset `index-WcSXKR_j.css`, `index-DBYHCncS.js` tertanam dalam build Go.
- Python unittest discovery `test_*.py`: 45 lulus (10 release preflight, 16 skill export, 19 recovery).
- Binary smoke lulus, fixture `.cache/tmp/smoke-ia040b9d`; recovery smoke lulus, fixture `.cache/tmp/recovery-smoke-5lbhrx_l`. Keduanya data sintetis terisolasi, bukan layanan/data pengguna.
- `git status --short`: bukan repository Git; tidak ada diff baseline, init, commit atau push.

Belum dibuktikan: browser nyata/mobile/keyboard, OpenCode–Hermes acceptance, wizard/rotasi, UI registry, helper retrieval terintegrasi, indeks persisted, pengukuran maksimum/busy-wait, serta aktivasi binary. Template layanan tidak dipasang, ekspor skill tidak dipasang ke harness, REL-05 tetap tertahan. Tidak ada restart/deployment/migrasi data aktif.

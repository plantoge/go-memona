# Frontend Svelte dan distribusi satu binary

## Arsitektur

Source Go dan Svelte berada dalam satu proyek, tetapi dikompilasi oleh tool masing-masing:

```text
frontend/src (Svelte + TypeScript)
    → Vite build
internal/webui/dist (HTML/CSS/JS)
    → go:embed + go build
bin/memona
    → satu proses Go menyajikan dashboard, /api dan /mcp
```

Database, Markdown, konfigurasi runtime dan secret **tidak** dimasukkan ke bundle/binary melalui direktori frontend. Tidak ada perubahan format data atau API pada migrasi ini. Node tidak dibutuhkan di server runtime apabila binary dibangun di mesin lain yang kompatibel.

## Build

Build frontend membutuhkan Node **22.18+ dalam lini 22, atau 24+** dan npm; Node 24 LTS disarankan. Versi Svelte/Vite/TypeScript dipatok di `frontend/package.json` dan transitif di `package-lock.json`. TypeScript 6 dipilih karena sesuai peer dependency svelte-check (bukan TypeScript 7).

Dari root:

```bash
make build
```

Urutannya: `npm ci --ignore-scripts` → type/accessibility check Svelte → Vite build → Go build. `--ignore-scripts` menghindari lifecycle script dependency; paket native opsional Vite tetap dipilih npm untuk platform build. Cache npm/Go berada di `.cache/`, tidak ada instalasi global. `make build` tidak me-restart proses aktif.

Target khusus:

```bash
make frontend-check
make frontend-test
make check
make build-go
```

`make build-go`, `make serve`, dan Go tests memakai `internal/webui/dist` yang tersedia, **tidak membangun Svelte otomatis**. Distribusi `dist` harus disertakan dalam source release untuk mendukung Go-only build. Untuk rilis UI terbaru gunakan `make build`, bukan hanya `go build`. Direktori dist bukan folder runtime yang disajikan dari disk; Go membaca embed.

Vite tidak melakukan penghapusan otomatis (`emptyOutDir: false`). Build berulang bisa menyisakan aset hash lama; review manifest/isi dist sebelum rilis. Penghapusan aset lama perlu review dan persetujuan, bukan cleanup seluruh proyek. Sourcemap production dinonaktifkan. Legacy `internal/webui/index.html`, `app.js`, dan `style.css` dipertahankan sebagai referensi dan tidak di-embed/disajikan.

## Development dua proses

Pilihan paling mendekati production: `make build` lalu jalankan binary dengan konfigurasi development tersendiri.

Untuk hot reload, jalankan backend dengan konfigurasi **data sintetis terpisah**, listener/public URL `http://127.0.0.1:8765`, cookie non-Secure; jangan gunakan direktori data yang sedang dibuka proses lain. Kemudian:

```bash
make frontend-install
make frontend-dev
```

Buka **http://127.0.0.1:5173**, bukan hostname lain. Vite bind loopback dan strictPort; `/api` diproxy ke `127.0.0.1:8765`. Host upstream disesuaikan dan hanya Origin persis `http://127.0.0.1:5173` yang ditulis ulang menjadi Origin upstream. Origin asing tidak ditulis ulang; Go tetap memeriksa Origin, Sec-Fetch-Site, sesi dan CSRF. `/mcp` tidak diproxy karena agen langsung menggunakan Go.

Tidak menambah CORS bebas atau origin development ke backend produksi. Jangan membuka Vite ke LAN/internet. Cookie Secure/konfigurasi HTTPS production tidak kompatibel dengan dev HTTP ini tanpa environment lokal tersendiri; jangan mengubah konfigurasi aktif untuk memaksanya.

## Keamanan dan UI

- Histori detail dashboard memuat **20 revisi per halaman** (lebih lama/terbaru), mengganti halaman agar konten tidak terus menumpuk. Diff hanya membandingkan revisi di halaman yang sama. Detail legacy tanpa `?history=paged` tetap kompatibel. Store masih memvalidasi semua revisi untuk mendeteksi korupsi; pagination bukan optimasi seluruh scan.
- Untuk skill nonarsip, riwayat menawarkan **Salin isi revisi … ke draf baru** dengan konfirmasi dan expected_revision. Hanya judul/isi/sumber disalin; relasi saat ini dipertahankan, histori tidak ditimpa, publikasi tidak otomatis. Hasil mutasi ambigu harus diperiksa dahulu sebelum retry karena endpoint ini tidak memakai operation_id.

- Editor create/revise menyediakan **Periksa catatan serupa**: request POST admin+CSRF hanya saat diminta, maksimum 10 metadata kandidat satu proyek, exclude ID yang diedit. Skor kemiripan leksikal bukan probabilitas duplikat. Tidak merge atau memblokir save; hasil lama tidak ditampilkan setelah teks berubah. Tidak ada kirim ke layanan eksternal. Ranking query dashboard kini sama dengan MCP: hasil substring utuh dipertahankan bila ada, fallback token jika kosong, lalu bobot judul > konten > sumber. Daftar tanpa query mempertahankan urutan sebelumnya dan filter arsip tetap tersedia.

- **Graph `3d-force-graph` (23 Oktober 2026, implementasi terbaru):** sesuai pilihan pengguna, renderer memakai [vasturiano/3d-force-graph](https://github.com/vasturiano/3d-force-graph), bukan renderer Three.js custom. Versi lokal dipatok `3d-force-graph@1.80.1`, `three@0.186.1`, dev `@types/three@0.186.0`; lifecycle install script dimatikan. `ThreeGraphCanvas.svelte` memuat renderer dinamis. Nodes/links disalin sebelum diberikan pada d3 agar mutasi posisi/endpoint engine tidak mengubah state/API. Node berwarna menurut jenis, ringkasan diperbesar, relasi garis tipis dengan supersedes oranye; panah hanya pada relasi node yang disorot. Tooltip bawaan HTML dinonaktifkan; label teks di-escape Svelte. Library upstream mencoba menginjeksi `<style>` saat import: CSP `style-src 'self'` tidak dilonggarkan dan dapat melaporkan pelanggaran style opsional tersebut. Styling yang dipakai wrapper disediakan dari CSS lokal; dependency/cache tidak diedit. Acceptance dengan CSP nyata tetap diperlukan. Revisi graph-first memperbesar kanvas mengikuti viewport, menyederhanakan heading dan toolbar, serta mengganti kontrol jeda/zoom/reset dengan ikon. Filter lanjutan ada dalam popover. Inspector, penjelajah daftar permanen dan bagian kontrol kamera tanpa gesture dihilangkan dari tampilan. Klik node langsung membuka dialog detail dengan isi memori sebelum metadata; render graph dijeda selama dialog terbuka. Pencarian menyediakan pilihan memori berbasis tombol untuk keyboard, dan jika WebGL gagal tersedia pemilih memori native. Keyboard kanvas tetap mendukung panah pan, Shift+panah rotasi, +/− zoom dan0 reset; tidak ada double-click-open.

  Orbit/zoom/pan dan drag node tersedia. Kualitas adaptif: >100 node ATAU >300 relasi mengaktifkan mode Ringan (DPR1, nodeResolution6, simulasi60 tick/1800ms), lainnya DPR maksimal1.5, nodeResolution12, simulasi120 tick/3000ms. Seluruh relasi memakai linkWidth0 (Three.Line), tanpa kurva kecuali self-loop; panah hanya untuk hubungan node hover/selected, warna tetap membedakan supersedes. Label default active saja; opsi label terdekat maksimal24 pada Ringan atau48 pada normal, scheduler trailing timer+RAF maksimal12.5/20Hz dan semua pekerjaan pending dibatalkan saat dispose. Tidak memproyeksikan semua500 label per tick. Reduced-motion memakai seed layout statis yang sudah dihitung, tanpa warmup d3 tambahan atau drag node; tidak ada auto-rotasi atau partikel berjalan. Jeda menghentikan render/interaksi kanvas; pilihan pencarian/tombol buka tetap tersedia. Halaman tersembunyi dijeda dan resource dilepas saat unmount. Loop render library tetap aktif saat tidak dijeda, meskipun force engine sudah dingin; bukan klaim render-on-demand. Maksimal 500 node visual (layout awal dibatasi sekitar2 juta interaksi pasangan), DPR adaptif, posisi tidak dipersistenkan dan kedalaman bukan hierarki. Tidak memakai CDN, jsonUrl atau layanan eksternal pada runtime. WebGL gagal/context-lost memberi fallback pemilih memori. Test helper/layout bukan bukti rendering/gesture GPU; acceptance browser/light-dark/mobile tetap pending karena browser/tools tidak tersedia. Warning ukuran chunk Vite tetap dilaporkan, tidak disembunyikan dengan menaikkan threshold.

- **Riwayat iterasi graph bergaya Obsidian (sebelum Three.js):** kanvas polos tanpa inspector permanen, node titik abu-abu (warna jenis opsional), garis lurus tipis, hover lingkungan dan label mengikuti zoom. Node dapat ditarik; helper `graph-physics.ts` menjalankan pegas/gaya tolak dengan jeda, reduced-motion, batas 600 langkah per restart dan cleanup saat unmount. Wheel zoom berjangkar posisi kursor. Panel detail/pengaturan muncul atas permintaan; arah relasi ada pada inspector/daftar teks. Posisi tidak dipersistenkan. Ini implementasi terinspirasi pola interaksi, bukan salinan engine Obsidian atau klaim kesetaraan visual. Acceptance browser masih pending; catatan implementasi statis sebelumnya di bawah menggambarkan tahap terdahulu.

- **Riwayat Knowledge Graph sebelum Three.js:** menggantikan tampilan kartu graf melalui `KnowledgeGraph.svelte` dan helper `knowledge-graph.ts`, tanpa dependency baru atau perubahan endpoint. Layout force-relaxation deterministik dibatasi 100 node, memprioritaskan ringkasan dan node dengan derajat terbesar dalam hasil yang dimuat. Anchor proyek hanya memengaruhi posisi; tidak membuat relasi tersirat. Filter jenis/relasi, inspector node, sorotan tetangga, fokus satu-hop, pan/zoom/reset dan daftar teks tersedia. Daftar teks mencakup hasil yang dimuat, bukan seluruh penyimpanan jika server membatasi hasil. Acceptance visual/keyboard/touch browser tetap belum dilakukan. Komponen `Graph.svelte` lama dipertahankan sebagai referensi, tidak dipakai oleh App.

- Kartu/detail key menampilkan identitas audit dan operasi memory/skill terakhir yang ditemukan dalam **200 event audit terbaru**. Endpoint `/api/key-usage` hanya admin; tidak mengirim token atau target catatan. Ini bukan last-use seluruh histori, status online, atau verifikasi aplikasi/perangkat. Jika kosong, UI tidak menyebut key belum pernah dipakai.

- Menu **Proyek** mengelola metadata registry lewat `/api/project-registry`: slug tetap, nama, deskripsi, alias dan arsip/pemulihan. Arsip metadata tidak mengubah catatan/key. Server tidak mengakses referensi lokasi. Edit memakai optimistic revision; konflik mempertahankan isian dan meminta reload.
- **Panduan integrasi** di MCP Server menyediakan snippet OpenCode V2/Hermes dengan placeholder environment, bukan token literal. Versi/profil hanya catatan lokal memori halaman. Pilihan proyek membantu form key, tanpa mengganti scope server. Tidak ada pengeditan konfigurasi klien otomatis.
- **Rotasi key** mengingat ID lama/pengganti selama halaman terbuka, meminta konfirmasi pengganti sudah diuji, lalu konfirmasi revoke lama. Ini pernyataan operator, bukan telemetry koneksi; last-use/online tidak dikarang. Token tetap sekali tampil dan tidak disisipkan ke snippet.

- Cookie HttpOnly dan CSRF tetap diatur Go; frontend tidak menyimpan sesi/key di localStorage.
- Token key baru hanya ada dalam memori UI, dibersihkan saat keluar/sesi habis/ditutup. Navigasi keluar dari halaman key meminta pengguna menyimpan token dahulu.
- Respons request yang datang setelah logout ditolak melalui generation guard; daftar/detail juga memiliki request guard agar respons lama tidak menimpa pilihan baru.
- HTML Markdown hanya dari renderer Go goldmark safe mode; source mentah tidak diberikan ke `{@html}`. Field lain di-escape Svelte.
- CSP backend tetap `script-src 'self'`, `style-src 'self'`, tanpa unsafe-inline/eval. Ikon memakai SVG lokal; animasi memakai stylesheet, bukan inline style.
- Hanya `/`, `/index.html` dan aset embed `/assets/` yang dilayani. Tidak ada akses direktori source/data atau fallback SPA untuk API/URL tidak dikenal.
- Sidebar floating memiliki dua ukuran: diperluas menampilkan ikon + label, diciutkan menjadi rail ikon yang tetap bisa diklik. Semua tombol memiliki accessible name dan tooltip native saat diciutkan. Pada layar kecil default rail; memilih menu menciutkan sidebar kembali.
- Tampilan memakai glass dengan token warna light/dark, fallback permukaan solid jika backdrop-filter tidak didukung. Header besar, tagline, dan footer dekoratif ditiadakan. Tombol refresh dan tema tetap tersedia sebagai ikon. Status rutin tetap diumumkan ke screen reader, error tetap terlihat.
- Pilihan dark/light melalui ikon bulan/matahari. Preferensi tema saja disimpan pada `localStorage` (`memona-theme`), bukan sesi atau key. Default mengikuti sistem; storage diblokir tetap tidak menggagalkan toggle.
- Menu **MCP Server** menampilkan endpoint konfigurasi server dan pengelolaan key melalui modal. Endpoint tidak mengandung key. Operasi key tetap create/read/revoke; membuat pengganti tidak mencabut key lama otomatis, dan tidak ada edit izin/hard delete baru.
- Menu **Penyimpanan** menampilkan laporan read-only admin dari `/api/storage-health`: kapasitas, integritas Markdown, staging, kegagalan write proses. Hasil lama dibersihkan saat refresh/logout/pindah menu; tidak ada polling, auto-repair atau penyimpanan laporan di browser. Hasil tidak lengkap/error tidak disebut sehat. Lihat [batas diagnostik](storage-health.md).
- Animasi menghormati prefers-reduced-motion. Browser/keyboard/mobile nyata tetap perlu acceptance.
- Detail memori menampilkan ringkasan proyek, terkait/menggantikan serta backlink dirujuk/digantikan oleh; tombol referensi membuka detail target. Form metadata menerima ID dalam proyek yang sama dan konfirmasi dampak supersedes. Penyimpanan memakai CSRF dan expected_revision, tanpa editor isi. Histori menampilkan metadata per revisi. Skill aktif kembali draft jika metadata direvisi. Lihat [relasi](memory-relations.md).

## Acceptance graph 3D yang masih diperlukan

Pada build baru dan data sintetis (jangan menyimpulkan dari helper test):

1. Buka Graph pada light/dark, uji hover/select, drag node, orbit latar, zoom/pinch, semua tombol kamera serta reset.
2. Uji filter proyek/jenis/relasi/arsip melalui toolbar/popover, node terisolasi/siklus dan arah related_to/supersedes. Perubahan filter tidak boleh membuka memori lain atau mengubah data tersimpan.
3. Periksa judul berisi `<img onerror=...>` tampil sebagai teks, tidak menjadi HTML tooltip; tidak ada request CDN/data eksternal. CSP harus tetap sama; catat warning style-injection upstream terpisah dari kegagalan graph.
4. Jeda/lanjutkan, pindah tab (visibility), reduced-motion sejak awal dan saat berubah, lalu pindah menu/logout. Periksa RAF/observer/konteks GPU tidak tertinggal setelah unmount.
5. Uji keyboard dan pembaca layar melalui pilihan hasil pencarian, ukuran375px dan label panjang. Popover harus bisa ditutup dengan Escape; fokus tidak tersembunyi. Klik node membuka isi dialog; tutup dialog kembali ke konteks graph.
6. Nonaktifkan WebGL/simulasikan context loss: fallback harus muncul dan pemilih memori/detail tetap bisa dipakai. Build/smoke HTTP tidak membuktikan skenario ini.

## Update production

Bangun dan uji binary dahulu. Pergantian binary/service dan restart produksi adalah langkah deployment terpisah yang memerlukan persetujuan. Data persisten di luar binary tidak ditimpa oleh build; ini bukan pengganti backup. Jangan menjalankan dua proses pada direktori data yang sama. Simpan versi binary sebelumnya dan tinjau kompatibilitas data sebelum rollback.

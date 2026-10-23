# Review interaksi knowledge graph

Tanggal: 22 Oktober 2026. Lingkup: `KnowledgeGraph.svelte`, helper `graph-physics.ts`, dan tes helper. Ini adalah review source dan verifikasi otomatis lokal, **bukan** acceptance browser atau deployment. Tidak ada dependency baru, perubahan server/auth, pembacaan data aktif, atau build/embed Go.

## Temuan dan perbaikan konservatif

1. **Fokus node tidak menghentikan simulasi.** Sebelumnya kamera hanya dipusatkan sekali saat keyboard memfokuskan node, tetapi posisi node terus berubah. Simulasi sekarang berhenti selama sebuah node mendapat fokus, dan dapat berjalan kembali setelah blur jika tidak dijeda/dibatasi preferensi gerakan. Enter/Spasi memilih node; Escape membatalkan pilihan. Pemusatan kamera dan kontrol pan/zoom tetap tersedia.
2. **Momentum drag bergantung pada frame physics.** Pada keadaan jeda atau reduced motion, drag dapat mengubah posisi tanpa menghapus kecepatan lama. Helper `moveParticle` sekarang mengubah posisi dan menghapus kedua sumbu kecepatan langsung, tanpa menunggu frame. Pin hanya mengikuti gesture aktif, bukan status permanen node.
3. **Capture pointer diarahkan ke SVG induk.** Klik setelah pointer capture dapat ditargetkan ke SVG alih-alih tombol node. Capture node sekarang berada pada elemen node; event move/up tetap bubbling ke SVG. Capture pan tetap pada SVG. Pointer lain tidak dapat mengambil alih gesture yang masih aktif; up/cancel dari pointer lain diabaikan.
4. **State klik sesudah drag/cancel tidak cukup terisolasi.** Penekanan klik kini terbatas pada klik pointer node yang baru diseret. `lostpointercapture` normal setelah `pointerup` tidak menimpa hasil gesture; cancel/lost capture aktif membersihkan gesture. Aktivasi keyboard dan tombol relasi tidak memakan flag klik lama. Gesture/capture dibersihkan saat filter mengganti layout, SVG menghilang, atau komponen dilepas.
5. **Penutupan pilihan dapat menghapus kontrol yang sedang mendapat fokus.** Tombol pembatalan mengembalikan fokus ke node terkait (fallback tombol Pengaturan). Pergantian data mengembalikan fokus panel ke Pengaturan, bukan node yang mungkin segera dihapus. Jika filter menutup inspector saat kontrol di dalamnya masih fokus, fokus dipulihkan ke Pengaturan setelah pembaruan DOM. Inspector tetap nonmodal; tidak ditambahkan focus trap.
6. **Beberapa effect berbagi kepemilikan frame.** Loop kini hanya dimiliki satu effect dengan disposer sendiri, diekstrak sebagai `startSimulation`. Dependensi sengaja terbatas pada seed, setelan, status jeda/fokus/reduced motion, keberadaan SVG/loading, dan versi restart eksplisit. Mutasi posisi partikel tidak dilacak sebagai pemicu effect. Disposer membatalkan frame dan menolak callback tertunda yang telanjur diteruskan. Loop tetap memiliki batas 600 langkah dan berhenti setelah minimal 60 langkah bila energi rendah.
7. **Reduced motion tidak tercermin pada kontrol.** Preferensi dibaca saat mount dan listener dilepas saat unmount. Animasi tidak dimulai sebelum preferensi tersedia. Saat reduced motion aktif tombol menunjukkan “Gerakan dikurangi” dan tidak menawarkan melanjutkan animasi yang memang diblokir. Drag langsung/pan/zoom tetap dapat digunakan; layout awal deterministik tetap dihitung seperti sebelumnya.

## Bukti otomatis

Dijalankan dari `frontend/` menggunakan dependency yang sudah tersedia:

- `npm run check`: **lulus**, 0 error, 0 warning Svelte/TypeScript.
- `npm test`: **lulus**, 34 tes, 0 gagal. Termasuk 5 tes graph physics: finite/overlap/self-edge dan pin, pembuangan momentum drag tanpa frame, satu frame/disposal termasuk callback tertunda, settle/batas 600/langkah dan pin dinamis, serta invariansi/clamp zoom.
- Pemeriksaan Git dicoba dari root proyek: direktori **bukan repository Git**, sehingga tidak tersedia `git diff`/status perubahan berbasis Git. Source akhir dibaca ulang; file agen lain tidak diedit.

Tes helper memakai scheduler frame palsu; ini memberi bukti cleanup dan batas loop helper, **bukan** bukti dispatch event pointer, reaktivitas DOM Svelte, fokus aktual, atau rendering browser. `npm test` juga mencakup tes frontend lain yang tersedia saat perintah dijalankan; tidak berarti seluruh fitur tersebut memperoleh acceptance browser.

## Acceptance yang masih perlu dijalankan di browser

Tool browser tidak tersedia pada sesi review; seluruh langkah berikut **belum dijalankan**:

- [ ] Klik/tap diam memilih node, klik ganda membuka memori, drag melewati ambang 4 px tidak memilih node secara tidak sengaja.
- [ ] Drag node saat simulasi berjalan, dijeda, dan reduced motion; lepas capture di luar peta; `pointercancel`/gesture sentuh terinterupsi; pointer kedua tidak mengambil alih.
- [ ] Setelah cancel/lost capture, Enter/Spasi pada node dan tombol relasi langsung bekerja, lalu klik pointer berikutnya tidak tertelan.
- [ ] Tab ke node menghentikan gerakan, ring fokus terlihat, Enter/Spasi memilih, Escape membatalkan; blur melanjutkan hanya bila diizinkan. Pastikan inspector tidak mengaburkan node fokus pada viewport sempit.
- [ ] Tutup pilihan dari panel, ubah filter sehingga node terpilih hilang, dan ganti data pencarian saat fokus di panel; fokus tidak jatuh ke body/elemen tersembunyi.
- [ ] Ubah preferensi reduced motion sewaktu halaman terbuka; pilihan jeda manual tetap dipertahankan. Navigasi pergi/kembali, loading, filter berulang dan mount/unmount tidak meninggalkan animasi/listener lama.
- [ ] Pan/zoom via tombol dan shortcut peta, daftar teks, layout 375 px/desktop, pembesaran teks/zoom browser dan kedua tema tetap terbaca tanpa overlap yang menghalangi kontrol.

## Batasan yang sengaja tidak didesain ulang

- Graf masih dibatasi 100 node oleh layout yang sudah ada; data/relasi eksplisit tidak diubah atau disimpulkan.
- Layout awal tetap relaksasi sinkron; review ini tidak menambahkan worker, dependency, atau algoritma layout baru.
- Efek hover/fokus/overlay, hasil screen reader, kompatibilitas capture SVG lintas browser, serta potensi klik ganda setelah dua gesture drag perlu acceptance nyata.
- Kontrol keyboard menyediakan pemilihan, pembukaan lewat panel/daftar, pan/zoom dan pemusatan; penempatan individual node lewat keyboard belum ditambahkan. Tidak ada klaim audit aksesibilitas/WCAG menyeluruh.
- Tidak menjalankan build, `make build-go`, deployment, restart layanan, atau mutasi data aktif. Perubahan source belum berarti binary aktif telah berubah.

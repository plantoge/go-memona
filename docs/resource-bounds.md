# Batas resource dan pembatalan — REL-06

**Status terbaru:** increment baca dan integrasi lanjutan; REL-06 belum selesai. Context SQL dan pre-staging mutasi kini tersambung. Algoritme autentikasi/format Markdown/dependency tetap; registry menambah tabel metadata saat binary baru dibuka. Tidak ada aktivasi atau perubahan data aktif. Arah SaaS bukan migrasi yang disetujui.

## Tambahan integrasi lanjutan

- Pagination key dashboard tersedia melalui `/api/keys-page`: 50 default/100 maksimum, keyset `created_at DESC,id ASC`, limit+1 baris untuk mendeteksi kelanjutan; audit diturunkan untuk key yang tampil saja. Endpoint legacy `/api/keys` dan `/api/key-usage` tetap kompatibel dan belum bounded dalam jumlah key. Tidak ada total count/snapshot lintas request atau indeks SQL baru; ini batas hasil/alokasi, bukan hard bound scan SQL. History tetap belum dipaginasi. Pernyataan baseline “key belum dipaginasi” di bawah berlaku untuk endpoint legacy, bukan jalur dashboard terbaru.

- Uji writer-lock SQLite nyata kini tersedia di `internal/access/busy_context_test.go`: audit deadline 40 ms selesai gagal sekitar 40,28 ms pada run lokal, tidak menyimpan event dan koneksi pulih. Ini bukan bukti semua operasi SQL memiliki hard deadline. Benchmark ranking/duplikasi snapshot 100/1000 catatan tercatat di verification; bukan benchmark scan maksimum atau delapan request bersamaan. Gap pagination/startup/encoding tetap terbuka.

- Login/Session/Authenticate/Logout/CreateKey/Revoke/Audit menerima context; wrapper kompatibel dipertahankan. API dan MCP memakai varian context. Audit setelah operasi memakai context background terpisah maksimal 2 detik, best-effort; ini bukan audit exactly-once dan dapat menambah waktu handler.
- Mutasi context membatasi mutex, pembacaan histori, validasi relasi/summary/cycle dan dirty-receipt rebuild dengan budget request. Rebuild mempublikasikan indeks hanya setelah lengkap. Startup dan wrapper legacy tetap tanpa budget tersebut.
- Cancellation diperiksa terakhir sebelum staging (atau fsync replay). Staging/rename/fsync berjalan tanpa interupsi context; disconnect **tidak membuktikan write batal**. Protokol receipt dan risiko commit ambigu tetap berlaku.
- Semua `/api/` dan `/mcp` mendapat context 5 detik. PBKDF2, syscall disk, encoding/render/sort serta commit tetap tidak preemptible; bukan hard deadline handler.
- JSON API dibuffer dan ditolak sebelum status sukses bila lebih dari 8 MiB. Encoder Go dapat mengalokasikan objek encoding internal sebelum cap buffer; ini bukan hard cap RSS. Output objek tool baca dibatasi 2 MiB sebelum SDK encoding; teks fallback/envelope SDK bukan termasuk cap tersebut. History/key belum dipaginasi.
- Limiter per-key 10/s burst 20, maksimum 4096 entri, idle 15 menit, ditambah limiter global lama. Bukan fairness ketat karena limiter global tetap dapat habis oleh satu klien.

Bagian “belum tercakup” di bawah merekam baseline tahap baca. Item context SQL, pre-staging mutasi dan cap output telah maju sebagaimana di atas; gap yang masih berlaku: startup legacy, SQLite busy-wait nyata, benchmark kapasitas/konkurensi besar, pagination, hard syscall/encoding bounds, audit retensi, dan acceptance browser/harness.

## Kontrak baru

- Request baca catatan memakai `ListContext`, `GetContext`, atau `HistoryContext`. Operasi tersebut mempunyai batas tetap **20.000 entri nama**, **256 MiB ukuran file revisi**, dan timeout kooperatif **5 detik**, termasuk waktu menunggu mutex. Context pemanggil yang lebih pendek tetap berlaku.
- Nama dibaca per batch maksimal 128, dengan paling banyak satu entri tambahan untuk mendeteksi limit. Budget List mencakup nama direktori catatan dan semua file histori, bukan hanya catatan yang cocok query atau diizinkan key. Filter izin tetap dilakukan sebelum informasi dikembalikan. Batas ini bukan kuota per tenant/key dan tidak menghilangkan biaya scan proyek lain.
- Batas byte diperiksa dari ukuran file sebelum pembacaan dan dibagi sepanjang satu operasi. Batas 1 MiB per revisi dan validasi canonical/transisi tetap berlaku. Perubahan ukuran selama baca dianggap korupsi; asumsi immutable/satu akun writer tetap diperlukan.
- **Tidak ada hasil parsial jika scan gagal/cancel/melewati budget.** History tetap oldest-first. Get/List memvalidasi seluruh histori tetapi hanya menyimpan revisi sebelumnya/latest, tidak menahan semua isi histori sekaligus. History tetap menahan semua revisi dalam budget.
- Menunggu mutex menggunakan `TryLock` + ticker 5 ms + context, tanpa goroutine pembantu yang dapat tertinggal. Setelah lock didapat, context dan status closed diperiksa kembali. Scan tetap memegang mutex agar snapshot konsisten; request baca dapat menunda writer sampai selesai/batal/batas tercapai.
- MCP search memakai satu snapshot untuk filtering dan relasi, bukan dua scan filesystem. Read/context/search/relations/skill menerima context handler. `toolContext` menggabungkan context SDK dengan context HTTP menggunakan `context.AfterFunc`; cancellation notifikasi SDK tetap dipertahankan, dan disconnect HTTP tidak bergantung pada versi protokol yang meneruskan context SDK. Tool baca mendapat deadline 5 detik untuk komputasi scan/filter/relasi; guide statis tidak melakukan scan.
- GET `/api/` mendapat context 5 detik; jalur notes/projects/detail menggunakan metode baca baru. Batas entry/byte berlaku **per panggilan store**, bukan total kumulatif seluruh request detail yang memanggil Get, History dan List. Deadline request dibagi bersama.
- `KeysContext` dan `ActivityContext` meneruskan context ke QueryContext serta memeriksanya selama koleksi. API listing key/activity memakainya; urutan dan limit activity 200 tidak berubah. Wrapper lama tetap tersedia.
- Derivasi relasi memeriksa cancellation antarkandidat dan menahan maksimal 20 referensi per kelompok (ID terkecil, deterministik), bukan mengumpulkan seluruh backlink sebelum dipotong. Flag `truncated` tetap berlaku. Admin notes membatasi 500 kandidat setelah sort sebelum menghitung relasi.

HTTP cancellation baca dipetakan ke **408** bila respons masih bisa dikirim; deadline/budget ke **503** + `Retry-After: 5`, tanpa path/isi/jumlah volume. MCP mengembalikan error tool aman, bukan hasil sukses terpotong. Retry tidak akan menyelesaikan koleksi yang selalu lebih besar dari budget; operator perlu meninjau kapasitas/desain, bukan loop retry agresif.

## Batas lama yang dipertahankan

- Admission HTTP **8 request global**, fail-fast 503 tanpa antrean; body **128 KiB**, termasuk batas SDK MCP. Ini bukan fairness atau kuota per-key.
- Login global burst 5/refill 12 detik; MCP global 30 request/detik burst 60.
- Search maksimal 50 hasil (default 20), excerpt 300 rune; context maksimal 12 dan anggaran seluruh hasil JSON terstruktur 24.000 byte (note, metadata/relasi, selection_reasons dan guidance). Itu **bukan** batas seluruh respons HTTP: SDK mengirim structuredContent serta fallback teks JSON.
- Inspector mempunyai budget terpisah; tidak digabung dengan scan request. Lihat [storage health](storage-health.md).

Angka 20.000/256 MiB/5 detik adalah guardrail konservatif awal yang sejajar dengan inspector, **bukan kapasitas maksimum teruji atau hasil optimasi benchmark**. Data besar yang sebelumnya dapat dibaca tanpa batas kini dapat ditolak oleh endpoint baca; startup dan mutasi tetap kompatibel. Pagination/index/budget yang dapat dikonfigurasi belum tersedia. Aktivasi binary tetap perlu review/backup dan persetujuan terpisah.

## Belum tercakup — alasan REL-06 tetap terbuka

1. Mutasi Markdown, validasi relasi/summary, rebuild receipt saat startup/dirty dan DFS cycle masih memakai jalur lama tanpa context/anggaran menyeluruh. Tidak ada cancellation baru pada staging/rename/fsync atau perubahan protokol commit. Disconnect saat mutasi tidak menjamin mutasi batal; gunakan operation_id dan arguments asli sesuai [idempotensi](idempotency.md).
2. SQL Login/Session/Authenticate, mutasi key/logout dan audit belum menerima context. Middleware auth sebelum endpoint, PBKDF2 dan audit sinkron setelah baca masih dapat menahan slot melewati deadline scan. Karena itu timeout 5 detik **bukan deadline wall-clock seluruh handler**. Hash PBKDF2 tidak dapat dihentikan di tengah.
3. Belum ada cap byte output global atau pagination history/key. Respons notes masih dapat besar; encoder/Markdown render/sort tidak seluruhnya preemptible. Pembatasan relasi mengurangi alokasi fan-in, bukan menghilangkan biaya scan berulang.
4. Syscall filesystem yang macet tidak dapat dihentikan paksa oleh context. Tidak ada jaminan hard real-time, proteksi DDoS, per-key fairness/tenant isolation, kuota storage, cleanup atau retensi otomatis.
5. Masih perlu test pembatalan transaksi/mutasi, SQLite busy wait, benchmark volume maksimum dan konkurensi scan delapan request, pengukuran wire MCP, serta acceptance browser/dua harness.

## Pengukuran sintetis awal

Go 1.26.8, Linux amd64, CPU Intel Core i3-10100T. `BenchmarkListContext`, 3 iterasi per skenario, konten 1 KiB/revisi, fixture canonical baru retained `.cache/tmp/read-bench-*`; setup fixture tidak masuk waktu ukur:

| Catatan × revisi | Waktu/op | Alokasi byte/op | Alokasi/op |
| --- | --- | --- | --- |
| 32 × 1 | 1,48 ms | 395.442 | 1.296 |
| 64 × 8 | 17,72 ms | 5.988.386 | 16.669 |

Log `.cache/tmp/resource-read-bench.log`. Angka ini satu run lokal dengan sampel kecil; bukan SLA, benchmark disk dingin, konten 64 KiB atau bukti batas maksimum aman. Alokasi kumulatif/op bukan peak RSS. Test budget menggunakan nilai awal counter mendekati batas untuk membuktikan exact/+1, bukan membaca 256 MiB atau membuat 20.000 file.

Bukti test aktual dan gap akhir ada pada [verifikasi](verification.md).

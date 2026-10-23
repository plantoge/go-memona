# Keamanan request dan batas operasional (SEC-02)

Implementasi source/test ini tidak berarti deployment atau restart sudah dilakukan. SEC-02 **belum selesai secara operasional**: keputusan retensi/growth audit append-only masih diperlukan. Tidak ada penghapusan, pruning otomatis, migrasi data aktif, atau dependency baru.

## Urutan admission

Host/origin/fetch-site, maksimum delapan request in-flight global, body 128 KiB, dan deadline request API/MCP tetap berlaku di boundary. Batas delapan mencakup autentikasi, audit kegagalan, request valid, dashboard, dan health; bukan pool delapan tambahan per kelas.

MCP memakai urutan berikut:

1. Bucket **direct peer** (60/detik, burst 120), lalu bucket pre-auth bersama (120/detik, burst 240). Penolakan peer tidak menghabiskan bucket pre-auth bersama.
2. Header wajib Bearer; autentikasi key selalu membaca store, tanpa cache validitas. Revocation/expiry tetap diperiksa setiap request.
3. Maksimum dua autentikasi mahal MCP/login bersamaan. Reservasi dilepas sebelum kerja terautentikasi, termasuk pada error/cancel. Ini subset batas delapan, bukan pelemahan batas global.
4. Kegagalan kredensial memakai bucket invalid terpisah (10/detik, burst 20): 401 selama tersedia, 429 setelah habis. Bucket ini **bukan** gate sebelum verifikasi key valid; key valid tidak ditolak hanya karena bucket invalid habis. Lookup kredensial yang tampak valid masih diperlukan untuk membedakan keduanya; pre-auth dan concurrency membatasi biaya lookup tersebut.
5. Key yang valid melewati bucket ID key (10/detik, burst 20), **baru** bucket MCP terautentikasi bersama (30/detik, burst 60). Request yang ditolak per-key tidak menguras bucket bersama. Request yang sudah lolos per-key tetapi ditolak bersama tetap membelanjakan token per-key (fail closed, tanpa refund).

429 menyertakan Retry-After 1 detik untuk MCP, sebuah petunjuk retry, bukan jaminan token/capacity sudah tersedia. Penolakan concurrency autentikasi memakai 503 + Retry-After 1; boundary delapan tetap 503 seperti sebelumnya.

Login mempertahankan aggregate burst 5, refill satu token/12 detik, ditambah bucket per direct peer dengan angka sama. Peer diperiksa lebih dahulu sehingga penolakannya tidak menguras aggregate. Aggregate tetap mencegah brute force terdistribusi. Body JSON invalid tetap membelanjakan token login seperti sebelumnya. Retry-After login tetap 12 detik.

## Identitas peer, tabel, dan keterbatasan fairness

Peer berasal **hanya dari RemoteAddr**, IP dikanonisasi (port diabaikan, IPv4-mapped IPv6 disamakan, zone IPv6 dilepas). Forwarded, X-Forwarded-For dan X-Real-IP tidak dipercaya. RemoteAddr tidak valid masuk satu bucket `unknown`, bukan entri tak terbatas. Alamat hanya ada dalam bucket memori, tidak masuk audit atau endpoint diagnostik.

Tabel key, peer MCP, dan peer login masing-masing maksimum 4096 entri. Idle 15 menit, lazy eviction dengan mutex/LRU, tanpa goroutine. Akses yang ditolak tetap memperbarui idle entri lama; entri aktif tidak digusur untuk memasukkan identitas baru. Saat penuh, identitas baru ditolak, bukan mereset bucket lama. Clock mundur tidak mencetak token baru. Semantik limiter lama tetap dipertahankan; hanya pemakai peer dan pembacaan jumlah agregat ditambah.

Ini **bukan fairness sempurna** atau perlindungan DDoS jaringan. Trafik invalid tidak menghabiskan bucket *terautentikasi*, tetapi masih dapat menghabiskan peer/pre-auth bersama, dua slot auth, delapan slot boundary, atau tabel peer. Banyak peer/distributed flood tetap dapat menolak request valid. Di belakang reverse proxy semua klien dengan RemoteAddr proxy yang sama berbagi bucket peer; spoofed forwarded header sengaja tidak menjadi bypass. Satu key tetap dapat memberi tekanan pada lookup/pre-auth walaupun ditolak per-key setelah autentikasi. Banyak key valid yang bersama-sama lolos kuotanya dapat menghabiskan bucket valid bersama. Tidak ada janji jatah individual ketika batas global sudah habis.

## Audit kegagalan dan observabilitas

Kegagalan kredensial MCP (termasuk header hilang) dan password login menghasilkan event tetap melalui AuditContext yang sudah ada: actor `anonymous`, action `auth.failure`, target kosong. Tidak menyimpan token, password, alamat, header, nama key, atau konten request. Request yang sudah ditolak admission/rate/concurrency dan kegagalan context bukan kegagalan kredensial yang diaudit.

Sampling kegagalan bersama MCP/login: burst 1, refill satu/menit, maksimum satu write in-flight, timeout maksimum 250 ms (context request yang lebih pendek/canceled tetap dihormati). Tidak ada antrean/goroutine audit background. Token sampling dikonsumsi sebelum reservasi write; saat reservasi penuh token tidak dikembalikan. Dengan demikian sampling adalah batas percobaan, bukan jaminan satu event tersimpan per menit. Restart mengisi burst baru. Audit aksi sukses/mutasi lama tidak memakai sampling ini.

`GET /api/request-security` hanya melalui middleware sesi admin yang sudah ada. Tidak ada UI baru. Endpoint berisi angka agregat process-local, batas tetap, dan peringatan retensi; tidak ada ID key/peer/token. Penghitungan tabel aman terhadap akses bersamaan. Snapshot beberapa counter bukan transaksi atomik; angka bisa berubah selama respons dibentuk. Current boundary mencakup request diagnostik itu sendiri.

Makna counter (reset ketika proses restart, bukan metrik durable):

- `auth_failures`: jumlah kegagalan kredensial yang mencapai tahap autentikasi (termasuk header MCP hilang).
- `rate_limited`: request MCP/login ditolak karena rate/tabel capacity (per-key, valid bersama, invalid, pre-auth, peer, atau login). Tidak mencakup host/origin atau boundary/concurrency.
- `auth_busy`: penolakan karena dua slot autentikasi sedang terpakai.
- `audit_sampled`: percobaan write audit kegagalan yang memperoleh token sampling dan slot; termasuk percobaan gagal.
- `audit_dropped`: kegagalan yang tidak dicoba ditulis karena sampling atau slot audit penuh, bukan record yang dihapus.
- `audit_errors`: percobaan write gagal/canceled/timeout; bukan retry otomatis. `audit_sampled - audit_errors` adalah jumlah write yang dilaporkan sukses oleh API selama proses ini, bukan hitungan seluruh record di database.

## Retensi: blocker operasional terbuka

Tabel activity SQLite tetap **append-only; tidak ada automatic purge**, batas ukuran file, atau job retensi baru. Batas 200 hasil API activity hanyalah jendela baca, **bukan** batas record tersimpan. Sampling hanya membatasi pertumbuhan *event kegagalan baru* (steady state maksimal sekitar 1440 percobaan/hari per proses ditambah burst restart); audit sukses/mutasi tetap dapat menambah record tanpa batas jangka panjang.

Sebelum SEC-02 dapat ditutup penuh, operator harus menyepakati anggaran kapasitas disk, pemantauan/alarm pertumbuhan, horizon retensi, backup konsisten, serta prosedur ekspor/arsip dan verifikasi pemulihan. Penghapusan atau perubahan data audit memerlukan persetujuan tersendiri; pekerjaan ini tidak melakukan atau mengusulkan pruning diam-diam. Counter bersifat lokal proses, jadi observasi lintas restart/replica memerlukan keputusan operasional tambahan.

## Bukti pengujian

`internal/server/security_test.go` menguji exhausted invalid budget lalu request valid melalui route SDK asli; noisy key tidak menguras budget key lain; spoof header dan canonical peer; bound pre-auth/auth concurrency/release; revoke/expiry tanpa cache; peer+aggregate login; audit redaction/sampling/slot/error; tabel peer maksimum 4096 dan idle eviction; serta admin-only endpoint/counter agregat. Test limiter key dan boundary delapan yang sudah ada tetap dipertahankan.

Pengujian menggunakan fixture sintetis dalam direktori temporer, bukan data aktif. Hasil command aktual dilaporkan terpisah; dokumen ini tidak mengklaim acceptance jaringan/proxy produksi atau deployment.

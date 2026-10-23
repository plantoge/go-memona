# Pembatasan request per API key

## Helper yang tersedia

`internal/server/key_limit.go` menyediakan `newKeyLimiter(maxEntries int, perSec rate.Limit, burst int, idle time.Duration)` dan `Allow(id string, now time.Time) bool`. Helper memakai token bucket dari dependensi `golang.org/x/time/rate` yang sudah ada.

- Identitas bucket harus **`key.ID` stabil hasil autentikasi**, bukan bearer token mentah, hash token, alamat IP, atau header `Forwarded`/`X-Forwarded-For`. ID kosong ditolak tanpa membuat entri.
- Setiap key mempunyai bucket sendiri. `Allow` mengonsumsi satu token; kekurangan token atau kapasitas tabel penuh menghasilkan `false`.
- Tabel dan daftar LRU dibatasi `maxEntries`, dilindungi mutex, dan dibersihkan secara lazy saat `Allow` dipanggil. Tidak ada goroutine, timer latar belakang, atau penyimpanan persisten.
- Hanya entri yang tidak diakses selama sedikitnya `idle` yang boleh dikeluarkan. Request ditolak karena token habis tetap memperbarui waktu akses. Entri aktif tidak dikeluarkan untuk memberi ruang kepada key baru: bila seluruh kapasitas aktif, key baru ditolak tanpa mereset bucket key lama.
- Urutan akses LRU menentukan urutan pembersihan entri idle; entri dengan waktu akses sama mengikuti urutan akses yang diserialisasi mutex. Timestamp mundur dijepit pada timestamp terakhir helper, sehingga tidak memundurkan TTL atau menggandakan refill token.
- Nilai kapasitas, laju, burst, atau idle nonpositif ditolak secara fail-closed. Perubahan jam maju tetap dapat mempercepat refill/expiry; integrasi sebaiknya memakai `time.Now()` yang membawa pembacaan jam monotonik di dalam proses.

Batas ini **process-local**, bukan kuota terdistribusi atau batas per proyek/tool. Restart proses mengosongkan tabel dan memberikan burst awal lagi. Beberapa proses memiliki bucket terpisah. Entri idle baru benar-benar dilepas pada pemanggilan berikutnya, tetapi jumlah entri tetap terbatas.

## Target integrasi endpoint MCP

Helper ini tidak mengubah autentikasi sendiri. Integrasi pada handler `/mcp` dikerjakan terpisah dengan parameter berikut:

| Lapisan | Pengaturan target |
| --- | --- |
| Limiter global yang sudah ada | Tetap 30 request/detik, burst 60; melindungi jalur sebelum autentikasi, termasuk request tanpa bearer atau bearer invalid. |
| Limiter per key | Maksimum 4096 entri, 10 request/detik per key, burst 20, idle 15 menit. |

Buat satu helper untuk setiap instance aplikasi, bukan setiap request. Setelah pemeriksaan bearer dan autentikasi berhasil, panggil `Allow(key.ID, time.Now())` sebelum meneruskan request ke handler MCP. Jika `false`, handler integrasi harus mengembalikan HTTP 429 tanpa menjalankan operasi MCP. Limiter global tetap berlaku; bucket per key tidak menjamin throughput bila lapisan global atau batas kerja server sudah penuh. Bearer invalid tidak boleh menghasilkan entri per-key.

Dokumen ini membedakan implementasi helper dari target wiring handler; keberadaan helper saja tidak membuktikan limiter per-key sudah dipanggil pada endpoint. Tidak ada klaim bahwa audit kegagalan autentikasi sudah diimplementasikan. Pembatasan request juga tidak menggantikan autentikasi, pemeriksaan scope/izin, Host/origin, CSRF, maupun batas ukuran body.

## Verifikasi

`internal/server/key_limit_test.go` menguji isolasi bucket, refill/burst, ID kosong, batas ukuran tabel, penolakan saat seluruh entri aktif, expiry pada batas TTL, pembaruan idle oleh request ditolak, urutan LRU, timestamp mundur, konfigurasi tidak valid, serta akses konkuren untuk burst dan kapasitas/expiry. Gunakan cache proyek dan `GOPROXY=off GOSUMDB=off` untuk pengujian tanpa unduhan modul; jalankan juga dengan race detector. Hasil pengujian helper bukan bukti acceptance endpoint, integrasi harness, atau deployment.

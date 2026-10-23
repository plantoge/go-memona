# Benchmark baca resource — Oktober 2026

## Ruang lingkup dan status

Pengukuran lokal **22 Oktober 2026**, pelengkap bukti REL-06 pada [batas resource](resource-bounds.md). Benchmark menambah profil koleksi/histori lebih besar, konten 64 KiB, serta delapan pemanggil `ListContext` pada **store yang sama**. Ini bukan penyelesaian seluruh REL-06, pengujian kapasitas maksimum, acceptance HTTP/MCP/browser, atau persetujuan aktivasi binary.

Perubahan kode hanya `internal/memory/read_bench_test.go`; laporan ini adalah berkas dokumentasi baru. Tidak mengubah source produksi, dependency, konfigurasi, permission, layanan atau data aktif. Direktori sintetis baru dibuat dengan mode yang sudah dipakai helper sebelumnya (0700/0600), tanpa chmod terhadap data yang ada. Tidak ada penghapusan fixture. Proyek bukan repository Git pada saat pemeriksaan (`git status --short` gagal dengan `not a git repository`), sehingga tidak ada diff/commit Git yang diklaim. Source lain sedang dikerjakan paralel; hasil berlaku pada snapshot source yang tersedia saat perintah dijalankan.

## Metode dan batas fixture

- Go **1.26.8**, Linux amd64, CPU **Intel Core i3-10100T @ 3,00 GHz** (identitas dari output benchmark). Tidak mematok afinitas CPU atau `GOMAXPROCS`, tidak mengisolasi mesin maupun beban agen lain.
- Fixture canonical sintetis baru dibuat langsung di `.cache/tmp/read-bench-*`, di luar timer benchmark, setelah membuka store kosong. Ini bukan pengukuran startup koleksi besar, rebuild receipt, throughput mutasi/fsync atau biaya membuat fixture.
- Semua catatan berproyek `benchmark`, berjenis `fact`, status `unverified`, tanpa relasi/receipt/arsip. Isi berulang `x`, ukuran tetap 1 atau 64 KiB per revisi; ini bentuk beban volume, **bukan dataset pengguna atau distribusi teks realistis**. Format canonical menyertakan isi juga dalam metadata JSON; byte file bukan sekadar ukuran isi.
- Helper menolak profil tidak valid sebelum menulis fixture: maksimum **10.000 entri nama** (direktori catatan + revisi), **96 MiB byte file canonical**, dan **64 KiB isi/revisi**. Estimasi byte memakai panjang encoded revision terbesar. Batas helper harus tetap di bawah konstanta request **20.000 entri / 256 MiB**; batas tersebut bukan kapasitas maksimum yang sudah diuji.
- Profil terbesar dalam jumlah entri memakai **9.000 entri**, sedangkan profil terbesar dalam byte memakai **67.253.760 byte** (~64,14 MiB). Keduanya tetap di bawah guardrail. Tidak mencoba exact-limit 20.000 entri/256 MiB atau memenuhi disk.
- Helper mengharuskan `GOTMPDIR`; tidak lagi fallback ke direktori test yang dibersihkan otomatis. Fixture **retained**, hanya store ditutup pada cleanup. Log menyimpan lokasi setiap fixture dan ukuran file aktual.
- Satu benchmark sequential operation = satu `ListContext`. Satu concurrent operation = **batch delapan panggilan**, barrier awal bersama dan channel hasil; semua hasil dikumpulkan sebelum iterasi selesai/benchmark gagal. Setiap panggilan wajib sukses dan menghasilkan jumlah catatan lengkap. Mutex store menserialisasi scan, sehingga delapan goroutine bukan delapan scan filesystem paralel. Waktu menunggu mutex termasuk timeout kooperatif store 5 detik.
- Benchmark memakai `b.Loop`, `ReportAllocs` dan timer di-reset setelah setup. Alokasi concurrent mencakup overhead goroutine/channel. Setup, penulisan fixture dan penutupan store tidak masuk ns/op; wall time perintah di bawah mencakupnya.
- Satu run, **3 iterasi per skenario** (`-count=1 -benchtime=3x`). Dipilih agar batch terberat tetap terbatas (tiga batch = 24 panggilan), tanpa kalibrasi durasi otomatis atau penggandaan fixture. Sampel terlalu kecil untuk statistik kapasitas, p95/p99 atau variasi lintas mesin. Fixture baru saja ditulis sehingga bukan benchmark cold-cache; tidak mengosongkan cache OS.
- Hanya fungsi `Benchmark...` memanggil helper. `go test` biasa tidak membuat fixture besar; menjalankan benchmark secara eksplisit tetap mengakumulasi fixture retained. Delapan fixture pada run ini berisi total **19.568 file revisi**, **4.352 direktori catatan**, dan **177.733.584 byte file canonical** (~169,50 MiB). Angka ini tidak mencakup overhead filesystem, direktori/lock maupun cache build. Pengulangan run akan menambah konsumsi lagi; tidak ada cleanup otomatis.

## Perintah dan bukti eksekusi

Dijalankan dari `/home/app/golang/memona`. Seluruh perintah Go memakai executable `/usr/local/go/bin/go` dan environment berikut:

```bash
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
export GOCACHE=/home/app/golang/memona/.cache/build
export GOMODCACHE=/home/app/golang/memona/.cache/mod
export GOPATH=/home/app/golang/memona/.cache/go
export GOTMPDIR=/home/app/golang/memona/.cache/tmp

/usr/local/go/bin/gofmt -w internal/memory/read_bench_test.go
/usr/local/go/bin/go test ./internal/memory -run '^$' -bench '^$' -count=1
/usr/local/go/bin/go version

TIMEFORMAT='wall_seconds=%3R user_seconds=%3U system_seconds=%3S'
{ time /usr/local/go/bin/go test ./internal/memory \
  -run '^$' -bench '^BenchmarkListContext$|^BenchmarkListContextConcurrent8$' \
  -benchtime=3x -count=1 -benchmem -v; } \
  > .cache/tmp/resource-read-bench-2026-10.log 2>&1
```

Go memakai cache proyek yang sudah ada; module proxy, checksum lookup dan toolchain download dimatikan. Compile dan benchmark berhasil offline tanpa instalasi/dependency baru. Perintah compile terarah (`-run '^$' -bench '^$'`) lulus (`ok ... 0.006s`), bukan eksekusi seluruh unit test. Percobaan awal menggunakan `/usr/bin/time` gagal karena binary tersebut tidak tersedia (exit 127, benchmark belum berjalan); diganti keyword `time` bawaan Bash tanpa instalasi. Log di atas memuat run yang berhasil.

## Hasil sequential

Setiap baris tiga iterasi, satu scan per operation. Byte canonical/entri berasal dari log helper. `B/op` adalah byte alokasi heap kumulatif, bukan byte yang dibaca atau peak memory.

| Catatan × revisi × isi/revisi | Entri nama | Byte canonical | ns/op | ms/op (~) | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 32 × 1 × 1 KiB | 64 | 74.592 | 4.357.974 | 4,36 | 395.453 | 1.296 |
| 64 × 8 × 1 KiB | 576 | 1.193.472 | 19.598.245 | 19,60 | 5.988.386 | 16.669 |
| 1.000 × 1 × 1 KiB | 2.000 | 2.331.000 | 56.990.896 | 56,99 | 12.349.186 | 40.110 |
| 1.000 × 8 × 1 KiB | 9.000 | 18.648.000 | 484.281.367 | 484,28 | 93.567.325 | 260.321 |
| 128 × 4 × 64 KiB | 640 | 67.253.760 | 442.877.100 | 442,88 | 302.137.432 | 23.846 |

## Hasil delapan pemanggil pada store yang sama

Setiap baris tiga iterasi, **delapan scan per operation**. Profil fixture sama dengan baris sequential terkait, tetapi menggunakan fixture/store baru sendiri. Angka **tidak boleh dibaca sebagai latensi atau alokasi satu request**. Tidak mengukur latensi masing-masing reader atau maksimum waktu tunggu; tidak membagi waktu batch menjadi klaim response-time per reader.

| Catatan × revisi × isi/revisi | ns/batch | detik/batch (~) | B/batch | allocs/batch | Hasil |
| --- | ---: | ---: | ---: | ---: | --- |
| 1.000 × 1 × 1 KiB | 429.798.821 | 0,430 | 98.797.920 | 320.922 | 24/24 snapshot lengkap |
| 1.000 × 8 × 1 KiB | 2.883.046.204 | 2,883 | 748.522.669 | 2.082.605 | 24/24 snapshot lengkap |
| 128 × 4 × 64 KiB | 4.328.070.551 | 4,328 | 2.417.284.824 | 191.184 | 24/24 snapshot lengkap |

Tidak ada error budget/deadline pada run ini; seluruh benchmark **PASS**. Waktu paket yang dilaporkan Go **27,586 detik**. Wall time seluruh perintah **27,973 detik**, CPU user **14,523 detik**, CPU system **4,047 detik**, dari Bash `time`. CPU times bukan CPU utilization masing-masing request dan bukan pengukuran peak RSS. Dua profil kecil diulang sebagai pembanding bentuk beban saja; perbedaan dengan angka historis tiga iterasi di dokumen lain tidak membuktikan regresi/perbaikan karena beban host/cache tidak dikontrol.

## Interpretasi dan gap yang tetap ada

1. Biaya scan/alokasi bertambah bersama jumlah revisi dan ukuran isi meskipun hasil hanya latest notes. Profil 64 KiB menunjukkan alokasi kumulatif besar; jangan menafsirkan guardrail byte input sebagai hard cap heap/RSS. Delapan batch readers tidak memerlukan delapan salinan fixture di disk, tetapi tiap scan tetap mengalokasikan hasil dan parser.
2. Sampel delapan pemanggil memberi bukti fungsional terbatas bahwa seluruh snapshot selesai pada beban sintetis ini, **bukan kapasitas maksimum, SLA, hard deadline, batas waktu terburuk, fairness mutex, atau jaminan delapan request pada data/mesin lain**. Rata-rata batch 4,328 detik tidak menentukan margin deadline tiap reader. Syscall macet tetap tidak dapat diputus paksa oleh context.
3. Tidak mengukur peak RSS, disk dingin, storage penuh, beban writer bersamaan, kontensi SQLite, auth/audit, startup/rebuild, receipt/relasi/cycle besar, banyak tenant, wire/encoding HTTP/MCP atau seluruh admission handler. Delapan panggilan store tidak setara dengan delapan HTTP request end-to-end.
4. Tidak ada benchmark eksplorasi hingga kegagalan/guardrail maksimum, profiling heap/RSS, pengulangan statistik, race run atau full suite pada langkah ini. Gofmt dan compile terarah serta run benchmark di atas adalah bukti yang benar-benar dijalankan. Parent perlu mengintegrasikan dengan verifikasi source lain dan tetap mempertahankan batas interpretasi ini.
5. Benchmark tidak memberi alasan menaikkan guardrail, mengubah runtime production, mengaktifkan service atau menghapus fixture. Operasi aktif, retensi/cleanup dan keputusan kapasitas tetap memerlukan review/persetujuan terpisah.

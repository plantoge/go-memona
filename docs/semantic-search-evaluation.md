# Evaluasi kecil pencarian leksikal dan parafrasa (RET-03 parsial)

## Status dan batas klaim

RET-03 **baru parsial**: tersedia baseline sintetis yang dapat diulang dan keputusan menunda embedding. Ini bukan evaluasi kualitas semantik produksi, bukan korpus pengguna aktual, bukan acceptance manual, dan bukan perbandingan model embedding. Tidak ada implementasi retrieval yang diubah.

Fixture di `internal/server/semantic_evaluation_test.go` dibuat khusus dari delapan catatan rekaan (enam aktif dalam scope, satu di luar scope, satu arsip). Sepuluh kueri memiliki label relevansi ID yang ditetapkan manusia berdasarkan maksud kueri, terpisah dari ekspektasi hasil implementasi. Dua kueri negatif tidak memiliki ID relevan. Penolakan proyek eksplisit diuji lagi secara terpisah. Fixture hanya memakai struct dalam memori: tidak membuka data catatan aktual, database, kredensial, layanan, atau model eksternal.

## Perilaku yang diuji

Berdasarkan `internal/server/retrieval.go`, `rankedNotes` memerlukan snapshot yang sudah diotorisasi, mengecualikan arsip, memecah teks menjadi token lowercase Unicode (underscore tetap bagian identifier), dan menerima kecocokan **salah satu** token. Bobot judul/konten/source adalah 6/2/1, ditambah bonus frasa berbatas token. Tidak ada stemming, kamus sinonim, terjemahan, atau embedding. Identifier parsial bukan token yang sama.

Berdasarkan `querySnapshot` di `internal/server/mcp.go` dan `matches` di `internal/server/server.go`, jalur pencarian memeriksa izin read, proyek, kind, dan arsip terlebih dahulu. Jika substring kueri penuh ditemukan pada judul/konten/source, hanya himpunan itu yang dikembalikan, lalu diurutkan secara leksikal. Bila substring parsial identifier tidak dapat diurutkan sebagai token, himpunan substring tetap dipertahankan dengan urutan tanpa kueri. Fallback any-term terhadap seluruh snapshot tersaring hanya terjadi jika tidak ada kecocokan substring penuh. Karena itu hasil `rankedNotes` langsung dan `querySnapshot` tidak selalu sama; test mengunci keduanya, bukan mengganti hasil aktual dengan hasil ideal.

## Fixture dan hasil aktual

ID aktif dalam proyek `demo`:

| ID | Judul | Konten |
| --- | --- | --- |
| `queue` | Antrian permintaan | Permintaan diproses berurutan. |
| `store` | Penyimpanan catatan | Simpan memori secara atomik. |
| `identifier` | Kontrak expected_revision | Revisi wajib cocok. |
| `safe` | Revisi aman | Perubahan diperiksa sebelum disimpan. |
| `alternate` | Aman dalam revisi | Validasi perubahan sebelum penulisan. |
| `conflict` | Revisi paralel | Konflik ditolak. |

`outside-scope` berada di proyek `other`; `archived` berada di `demo` tetapi diarsipkan. Keduanya berjudul “Deployment rahasia”. Key rekaan hanya memiliki read untuk `demo`. Snapshot `rankedNotes` disaring menurut key sebelum pemanggilan; `querySnapshot` menerima kedelapan catatan untuk memeriksa penyaringan mandirinya. Semua field yang tidak dicantumkan memakai zero value; ini fixture fungsi retrieval, bukan fixture validasi penulisan atau penyimpanan.

Urutan ID di bawah adalah urutan yang diasersikan oleh test, bukan sekadar himpunan:

| Kueri | ID relevan berlabel manusia | `rankedNotes` | `querySnapshot` |
| --- | --- | --- | --- |
| `antrean` | queue | kosong | kosong |
| `persist note` | store | kosong | kosong |
| `atomic write` | store | kosong | kosong |
| `expected_revision` | identifier | identifier | identifier |
| `expected_rev` | identifier | kosong | identifier |
| `revisi aman` | safe, alternate | safe, alternate, conflict, identifier | safe |
| `konflik revisi` | conflict | conflict, alternate, safe, identifier | conflict, alternate, safe, identifier |
| `antrian` | queue | queue | queue |
| `galaksi kuasar` | tidak ada | kosong | kosong |
| `deployment rahasia` | tidak ada dalam scope aktif | kosong | kosong |

Kueri eksplisit `{project: "other", query: "deployment rahasia"}` juga harus menghasilkan `access.ErrDenied` tanpa catatan, bukan dianggap kegagalan recall. Catatan di luar scope dan arsip tidak dimasukkan sebagai label relevan yang seharusnya ditemukan.

### Hitungan kecil, bukan estimasi korpus pengguna

Ada **9 pasangan kueri–ID relevan** dari delapan kueri positif. Micro recall dihitung dari ID relevan yang muncul di **seluruh hasil** dibagi 9, bukan recall@k. Dua kueri negatif mempunyai recall tidak terdefinisi dan tidak menambah denominator.

- `rankedNotes`: **5/9 (55,6%)**, total 10 hasil, termasuk 5 hasil tidak berlabel relevan.
- `querySnapshot`: **5/9 (55,6%)**, total 8 hasil, termasuk 3 hasil tidak berlabel relevan.
- Kedua kueri negatif: kosong pada kedua jalur; proyek eksplisit di luar scope: ditolak.

Angka recall sama **tidak berarti perilaku sama**: `querySnapshot` menemukan identifier parsial tetapi kehilangan `alternate` saat substring `revisi aman` sudah ditemukan. Kedua jalur melewatkan tiga sinonim/parafrasa tanpa overlap token; any-term juga membawa hasil tambahan yang tidak relevan menurut label. Miss ini disengaja untuk mencatat keterbatasan aktual, bukan dihapus dari test agar terlihat baik. Label, ukuran, dan pilihan kueri sangat terbatas serta sengaja menonjolkan kasus tertentu; angka ini tidak dapat digeneralisasikan ke trafik atau catatan nyata.

## Verifikasi offline

Toolchain yang diverifikasi: `go version go1.26.8 linux/amd64`. Jalankan dari root proyek dengan cache yang sudah tersedia; `GOPROXY=off`, `GOSUMDB=off`, dan `GOTOOLCHAIN=local` mencegah pengunduhan modul/toolchain oleh Go. Jika cache tidak lengkap, berhenti dan laporkan kegagalan; jangan menginstal atau mengubah dependensi untuk melewatinya.

```bash
env GOCACHE="$PWD/.cache/build" \
  GOMODCACHE="$PWD/.cache/mod" \
  GOPATH="$PWD/.cache/go" \
  GOTMPDIR="$PWD/.cache/tmp" \
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
  /usr/local/go/bin/go test ./internal/server \
  -run '^TestSyntheticSemanticEvaluation$' -count=1 -v
```

Eksekusi awal test fokus tersebut lulus pada Go 1.26.8 menggunakan cache proyek dan menghasilkan hitungan di atas. Eksekusi ulang setelah perapian log terblokir kompilasi pada source di luar kedua file evaluasi: `internal/memory/store.go:61:20` dan `:76:20`, `undefined: SkillMetadata`. Definisi itu tidak diperbaiki oleh pekerjaan evaluasi ini; perlu verifikasi ulang setelah perubahan source terkait lengkap. Test tidak memerlukan dependency baru, konten eksternal, pemanggilan model, atau layanan yang berjalan. Ini bukan bukti seluruh suite, UI, deployment, performa, maupun kualitas semantik produksi telah lolos.

## Keputusan: tunda embedding

**Jangan menambahkan embedding sekarang.** Pertahankan baseline ini dan tunggu dataset representatif yang disetujui pemilik data beserta label relevansi, kebijakan penggunaan, dan target keberhasilan. Kesenjangan leksikal terlihat pada contoh rekaan, tetapi tidak membuktikan bahwa embedding tertentu meningkatkan manfaat nyata atau aman diterapkan.

Sebelum keputusan berikutnya:

1. **Dataset dan kriteria:** setujui contoh Indonesia/kode yang representatif, anonimisasi/redaksi, label relevansi termasuk hard negatives dan identifier, pemisahan pengembangan/evaluasi, serta metrik recall@k/precision@k dan latensi yang ditentukan sebelumnya. Jangan membaca atau mengekspor catatan pengguna tanpa persetujuan.
2. **Privasi dan scope:** pertimbangkan sensitivitas teks maupun vektor, retensi, backup, penghapusan/revisi/arsip, dan risiko kebocoran lintas proyek. Penyaringan izin harus tetap berlaku sebelum kandidat atau konten dapat terungkap. Penyedia eksternal memerlukan persetujuan khusus untuk pengiriman data, kebijakan penyimpanan, dan lokasi pemrosesan; test ini tidak melakukan pengiriman itu.
3. **CPU dan latensi:** ukur tokenisasi, inferensi lokal, batch pengindeksan ulang, concurrency, dan dampak pada request biasa. Model lokal pun dapat menambah beban CPU dan waktu startup; belum ada pengukuran dalam evaluasi ini.
4. **Memori dan disk:** ukur RAM model, dimensi vektor, ukuran indeks, pertumbuhan korpus, proses pembaruan revisi, serta kebutuhan backup. Jangan menyimpulkan kebutuhan memori dari delapan catatan ini.
5. **Biaya dan operasi:** bandingkan biaya inferensi lokal dengan biaya per-token/request eksternal, pengindeksan ulang, penyimpanan, observabilitas, lisensi model, dan pemeliharaan. Belum ada model, harga, atau estimasi biaya yang divalidasi di sini.
6. **Eksperimen aman berikutnya:** setelah persetujuan dataset dan sumber model, bandingkan lexical baseline, opsi lexical yang terarah, dan semantic/hybrid secara offline di data evaluasi yang sama; simpan jalur rollback dan jangan melemahkan test scope atau kompatibilitas identifier demi skor. Perubahan auth, deployment, instalasi, dan akses data aktual tetap di luar pekerjaan ini.

RET-03 tetap terbuka untuk evaluasi dataset representatif, pengukuran sumber daya/biaya, dan keputusan implementasi. Tidak ada klaim bahwa semantic search sudah tersedia atau layak produksi.

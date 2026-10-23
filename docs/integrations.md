# Integrasi MCP dan acceptance

## Kontrak tool

Endpoint MCP: `/mcp`, transport HTTP dengan `Authorization: Bearer …`. Browser admin menggunakan sesi password yang berbeda. Setiap operasi harus dibatasi proyek di server; input `project` dari agen bukan bukti otorisasi.

Panduan agen dapat dibaca melalui `get_memory_guide({})` atau resource `memona://guide`; lihat [discovery panduan](agent-guide.md). Setelah upgrade dan restart terdapat sepuluh tool.

| Tool | Izin minimum | Tujuan |
| --- | --- | --- |
| `get_memory_guide` | API key valid | Aturan baca/tulis dan contoh, tanpa data proyek |
| `get_context` | `read` | Ringkasan konteks proyek yang diizinkan |
| `search_memory` | `read` | Pencarian substring teks pada scope yang diizinkan |
| `read_memory` | `read` | Membaca catatan terbaru berdasarkan ID, termasuk arsip, sesuai scope |
| `get_memory_relations` | `read` | Referensi satu-hop terkait/menggantikan dan backlink, maks. 20 per jenis, tanpa isi target |
| `save_memory` | `write` | Membuat memori; skill dari agen harus draft |
| `revise_memory` | `write` | Revisi dengan `expected_revision`, tanpa menimpa revisi lama |
| `save_handoff` | `write` | Menyimpan keadaan akhir sesi dan langkah selanjutnya |
| `archive_memory` | `archive` | Mengarsipkan, bukan hard delete; konflik versi diperiksa |
| `get_skill` | `read` | Mengambil objek catatan skill berstatus `active`, nonarsip, berdasarkan ID |
| `find_duplicate_candidates` | `read` | Maksimal sepuluh metadata kandidat leksikal satu proyek eksplisit; tanpa merge/write |

### Argumen sesuai source saat ini

Skema SDK diturunkan dari struct di `internal/server/mcp.go` dan `memory.Input`. Cocokkan `tools/list` pada build yang diuji; peninjauan ini bukan hasil discovery runtime.

| Tool | Field wajib | Field opsional / perilaku |
| --- | --- | --- |
| `search_memory` | Tidak ada | `project`, `query`, `kind`, `limit`; project kosong mencari semua proyek yang diizinkan. Limit 1–50, selain itu default 20. |
| `get_context` | Tidak ada | `project`, `query`, `kind`, `limit`, `include_testing`; project kosong berarti `global`. Limit 1–12, default 12; anggaran seluruh hasil JSON terstruktur 24.000 byte termasuk guidance/selection_reasons. Testing eksplisit dikecualikan default; mixed maksimal dua handoff, kind=handoff mengabaikan kuota kategori. Ringkasan diprioritaskan dan melewati filter query, tetapi tidak kind. Catatan digantikan nonarsip dikecualikan. |
| `read_memory`, `get_skill`, `get_memory_relations` | `id` | Tidak menerima nomor revisi historis. Read/skill menyertakan `connections`; relations hanya referensi. |
| `save_memory`, `save_handoff` | `project`, `kind`, `title`, `content`, `status`, `source` | `save_handoff` tetap memakai skema penuh, lalu memaksa kind menjadi `handoff`. |
| `revise_memory` | `id`, `expected_revision`, dan keenam field `memory.Input` di atas | Penggantian lengkap, bukan patch parsial; memeriksa scope proyek asal dan tujuan. |
| `archive_memory` | `id`, `expected_revision` | Tidak menerima isi pengganti. |
| `find_duplicate_candidates` | `project`, `title`, `content` | `exclude_id` opsional; proyek eksplisit wajib berizin read, maksimum sepuluh metadata nonarsip, bukan probabilitas duplicate. |

Keenam field inti `memory.Input` wajib hadir pada skema meskipun `source` boleh berupa string kosong untuk status selain `verified`; `content` juga boleh kosong. `project` harus cocok `^[a-z0-9][a-z0-9-]{0,63}$`, judul wajib tidak kosong (maksimum 200 byte), konten maksimum 64 KiB, dan sumber maksimum 2.000 byte. Jenis storage: `fact`, `decision`, `knowledge`, `handoff`, `skill`. Status input: `unverified`, `verified`, atau `draft`; `verified` memerlukan sumber tidak kosong. Skill baru wajib `draft`. Status `active` hanya hasil publish admin; revisi skill aktif kembali draft.

Field **opsional**: `is_project_summary`, `related_to`, `supersedes`. Saat revise, field yang tidak dikirim dipertahankan; `false`/`[]` mengosongkannya. Penetapan metadata memerlukan read+write. Ringkasan hanya knowledge, maks. 8000 byte, satu nonarsip per proyek. Target relasi harus nyata dalam proyek yang sama, unik, maks. 20 per jenis; diri sendiri/siklus supersedes ditolak. Backlink turunan hanya dari sumber nonarsip dan dibatasi. Search menyertakan `superseded_by`; read/skill/context memakai `connections`. Lihat [kontrak relasi](memory-relations.md), termasuk batas rollback format.

**Acceptance pengguna 23 Oktober 2026:** OpenCode dilaporkan sudah dicoba. Hasil ini tidak membuktikan versi Hermes/harness lain, browser seluruh fitur atau aktivasi binary increment terbaru.

Metadata review opsional non-skill: `verified_at` waktu UTC RFC3339Nano kanonis yang dinyatakan penulis (status verified + source evidence), `needs_review` flag keraguan eksplisit. Omission mempertahankan; string kosong/false membersihkan; keluar dari verified menghapus waktu warisan. Search/read/context menampilkannya, context memberi warning tanpa menyelesaikan konflik berdasarkan recency. [Kontrak lengkap](review-metadata.md).

Metadata tambahan opsional: `is_testing` boolean (omission revise mempertahankan, false membersihkan) dan `skill_metadata` khusus skill (trigger/version/compatibility/test_evidence; omission mempertahankan, objek kosong membersihkan). Lihat [testing](testing-metadata.md) dan [metadata skill](skill-metadata.md) untuk batas byte/rollback. Tidak ada klasifikasi otomatis atau bukti test yang diverifikasi server.

Respons catatan memiliki `revision`; gunakan nilainya sebagai `expected_revision` pada mutasi berikutnya. Tidak ada alias field versi lain, tool hard delete, tool publish admin, atau eksekusi skrip.

Empat mutation tool `save_memory`/`save_handoff`/`revise_memory`/`archive_memory` menerima `operation_id` opsional untuk retry aman. Namespace per key dan tool, ID 8–128 karakter ASCII (pertama alfanumerik, sisanya alfanumerik/`._:-`). Kirim arguments asli dengan ID yang sama setelah timeout; payload berbeda ditolak. Replay mengembalikan revisi awal, bukan latest; identitas dipertahankan bersama histori tanpa TTL. Tanpa ID/string kosong tidak ada deduplikasi. Tidak ada field baru pada API admin; autentikasi/scope dan redaksi tetap berlaku. Rincian fingerprint, retensi, crash/restore, audit serta rollback pada [idempotensi](idempotency.md).

### Jika harness menolak respons karena output schema

Setelah upgrade relasi, respons memiliki metadata dan `connections` yang belum ada pada kontrak lama. Bila harness melaporkan `Structured content does not match the tool's output schema` atau `additional properties`, bandingkan discovery `tools/list` dari endpoint/build yang benar dengan skema yang dikenali harness. Katalog delapan tool tanpa `get_memory_relations`/`get_memory_guide` dapat menunjukkan kontrak lama, bukan bukti pasti bug server. Reconnect MCP atau mulai sesi baru sesuai mekanisme harness untuk memperbarui discovery; jangan menonaktifkan validasi atau mengubah data/secret untuk menyiasatinya. Pastikan dulu binary yang dimaksud sudah diaktifkan dengan persetujuan; reconnect saja tidak mengganti binary server.

`TestMCPDiscoveredOutputSchemas` memvalidasi structured output sebelas tool terhadap skema runtime `tools/list`, termasuk hasil kosong, metadata, hubungan dan mutasi sintetis. Test lokal ini lulus, tetapi bukan bukti bahwa cache/koneksi OpenCode atau Hermes pengguna telah diperbarui. Reconnect/acceptance nyata tetap dicatat terpisah.

Contoh **arguments** untuk `save_memory` (bukan envelope JSON-RPC):

```json
{
  "project": "acceptance-demo",
  "kind": "decision",
  "title": "MEMONA-ACCEPT-run-001",
  "content": "Pilihan awal A, belum diverifikasi.",
  "status": "unverified",
  "source": ""
}
```

Untuk `revise_memory`, sertakan enam field yang sama dengan konten baru, lalu `id` nyata hasil save dan `expected_revision` hasil read. Untuk `save_handoff`, kirim keenam field dengan `kind: "handoff"`; jangan hanya mengirim teks ringkasan.

Panduan: [OpenCode V2](../integrations/opencode-memory.md) dan [Hermes](../integrations/hermes-memory.md). Jangan menimpa konfigurasi harness yang sudah ada. Key terpisah memudahkan audit dan revoke; jangan memakai key admin browser sebagai bearer.

## Persiapan pengujian yang dapat direproduksi

Semua langkah di bawah **manual pending** sampai ada bukti aktual. Gunakan data sintetis, bukan data pribadi atau secret.

1. Catat tanggal, versi Go, versi aplikasi/commit/hash binary, versi OpenCode V2 dan Hermes, OS, URL, serta konfigurasi nonsecret. Jalankan `make test`, `make race`, `make vet`, `make build`; simpan exit code dan hasil yang telah disunting dari secret.
2. Siapkan konfigurasi/data baru yang terisolasi dan disetujui. Uji bootstrap tanpa password dan dengan password kurang dari 16 karakter: startup harus gagal. Masukkan password valid lewat prompt tersembunyi dan jalankan `memona serve` sesuai README.
3. Login UI; gunakan proyek sintetis `acceptance-demo` dan `acceptance-other`. Buat key berbeda untuk OpenCode dan Hermes, scope hanya `acceptance-demo`, izin `read`, `write`, `archive`. Buat juga key read-only untuk uji negatif. Tidak perlu scope `*` untuk skenario ini.
4. Gabungkan snippet integrasi pada konfigurasi/profil yang benar, tanpa menghapus konfigurasi lain. Pastikan dua harness melakukan discovery pada `/mcp` dan menemukan sepuluh tool di atas. Catat nama wrapper aktual dan skema, tanpa key.
5. Gunakan penanda sintetis unik, misalnya `MEMONA-ACCEPT-<tanggal>-<run-id>`, agar hasil tidak tertukar dengan percobaan sebelumnya. Jangan membersihkan data uji dengan hard delete; arsipkan sesuai izin bila selesai.

## Skenario inti: OpenCode → UI → Hermes → OpenCode

| Langkah | Aksi yang dapat diulang | Bukti / hasil yang diharapkan |
| --- | --- | --- |
| 1 | Di OpenCode, panggil `save_memory` untuk keputusan proyek `acceptance-demo` berjudul penanda uji; isi: “Pilihan awal A, belum diverifikasi.” Gunakan skema hasil discovery. | Catat ID dan versi; UI menampilkan proyek, isi, penulis, status dan revisi awal yang sama. Jangan menganggap jawaban teks agen sebagai bukti panggilan tool. |
| 2 | Tinjau catatan di UI (read-only), lalu di OpenCode panggil `read_memory` dan `revise_memory` dengan `expected_revision` terbaru serta seluruh field pengganti; isi baru “Pilihan B setelah review.” Muat ulang detail UI. | ID tetap, nomor revisi naik, revisi lama terlihat di riwayat UI; catat versi terbaru. UI tidak menyediakan editor revisi. |
| 3 | Di sesi Hermes baru, `get_context`, `search_memory` dengan penanda, lalu `read_memory` berdasarkan ID. | Hermes mendapatkan pilihan B pada scope yang benar, bukan hanya salinan percakapan OpenCode. Cocokkan hasil tool aktual. |
| 4 | Di Hermes, simpan `save_handoff` dengan ID catatan sebagai referensi dan langkah berikut “Uji pilihan B”. Kembali ke OpenCode dalam sesi baru dan cari/baca handoff itu. | Handoff yang dibuat Hermes terlihat dari OpenCode dan UI; identitas key/aktor sesuai audit. |
| 5 | Di UI, tinjau histori/audit. Restart Memona secara normal tanpa bootstrap ulang, lalu ulangi baca dari kedua harness. | Revisi dan handoff bertahan; admin lama tidak berubah; tidak ada token/password dalam bukti. |

**Tidak ada satu pun langkah tabel ini yang dinyatakan telah lulus oleh dokumentasi ini.** Setelah menjalankan, tulis hasil per langkah: pass/fail/blocked, versi, ID sintetis, dan bukti yang disunting. Koneksi MCP saja bukan uji end-to-end.

## Uji negatif dan lifecycle (manual pending)

- **Proyek:** key scope `acceptance-demo` mencoba search/context `acceptance-other` dan baca/revisi ID miliknya. Semua harus ditolak/tidak membocorkan isi. Jika revisi memungkinkan pindah proyek, uji asal dan tujuan. Uji `get_skill` dan handoff dengan batas yang sama.
- **Izin:** key read-only berhasil baca, gagal save/revise/handoff/archive. Key write tanpa archive gagal archive; key tanpa read tidak boleh mengambil konteks. Otorisasi bukan sekadar tools yang disembunyikan klien.
- **Konflik:** baca versi N dari dua klien. Klien A merevisi dengan N; klien B memakai N harus konflik. Baca ulang, gabungkan secara sadar, lalu revisi dengan versi terbaru.
- **Revoke:** panggil tool dengan key valid, cabut lewat UI, lalu panggil lagi dari klien yang masih berjalan dan klien baru. Keduanya harus ditolak pada request berikutnya; server MCP stateless, tidak memerlukan penutupan sesi transport. Key invalid/tanpa bearer juga ditolak. Secret tidak muncul kembali di daftar key atau log; database hanya hash.
- **Arsip:** arsipkan catatan dengan versi terbaru; konteks/search aktif tidak memakainya. UI masih bisa meninjau arsip/riwayat sesuai jalur implementasi. Tidak ada hard delete. Admin dapat memulihkan melalui dashboard; pastikan revisi baru tercatat dan catatan kembali tersedia. `archive_memory` hanya mengembalikan receipt ID/revisi/status arsip, bukan isi catatan, agar izin archive tidak memberikan akses read.
- **Skill:** agen membuat skill draft; `get_skill` harus menolaknya sampai aktif. Admin meninjau dan publish melalui UI, lalu kedua harness mengambil versi yang dipublikasikan. Revisi skill aktif kembali draft dan perlu review baru. Pastikan agen tidak bisa publish atau menjalankan skrip. Tombol publish hanya tersedia pada skill draft nonarsip. Verifikasi visibilitas tombol dan pesan konflik melalui browser nyata.
- **Bootstrap ulang:** restart dengan variabel bootstrap berbeda, pastikan password admin lama tetap berlaku dan nilai baru tidak menggantikannya; setelah itu hilangkan variabel bootstrap dari lingkungan operasi rutin.
- **Lock dan persistensi:** proses kedua dengan direktori data yang sama harus gagal karena lock; jangan menghapus lock. Setelah stop normal proses pertama, restart dan pastikan data utuh.

## Browser, backup, dan publik (manual pending)

- Login/logout, penolakan login salah, kedaluwarsa sesi 8 jam, dan kontrol akses sebelum login. Uji limiter login global burst 5/refill satu token per 12 detik serta MCP global 30 request/detik burst 60 pada lingkungan uji terisolasi, bukan layanan pengguna; pastikan HTTP 429 ditangani tanpa retry agresif.
- Cookie HttpOnly/SameSite Strict; pada HTTPS publik juga Secure. Mutasi tanpa token CSRF atau dari origin/host tak diizinkan ditolak. UI tetap berfungsi pada origin yang benar.
- Markdown berbahaya tidak mengeksekusi JavaScript; input traversal/symlink dan payload tak valid ditolak tanpa membocorkan path/secret. Jangan menguji dengan data pengguna.
- Backup setelah stop dan restore ke direktori baru mengikuti [operasi](operations.md), termasuk admin, key/revoke, audit, revisi, dan skill. Catat hasil restore nyata, bukan hanya keberhasilan `tar`.
- Layout, navigasi keyboard, pesan konflik/error dan penampilan secret hanya sekali diperiksa di browser nyata.
- HTTPS/reverse proxy/Cloudflare Access hanya diuji setelah exposure disetujui. Pastikan MCP mendapat respons protokol, bukan halaman login HTML; autentikasi bearer Memona tetap wajib.
- `get_skill` mengembalikan catatan `kind: "skill"` dan teks prosedur, bukan paket berkas. Tidak ada `data/skills`, registri native, auto-install, atau katalog HTTP Memona. Adaptasi native manual bersifat opsional di luar aplikasi dan diuji terpisah.

## Matriks pelaporan awal

| Area | Bukti saat penulisan dokumen |
| --- | --- |
| Manifest, CLI, storage, access, server/MCP, dan UI JavaScript | Ditinjau statis; bukan hasil test |
| Test Go, race, vet, build | Lulus pengujian pengembangan; lihat [verifikasi](verification.md) |
| Lima langkah lintas harness + UI | Manual pending |
| Uji otorisasi, key revoke, CSRF, XSS | Test otomatis lulus; browser/publik manual pending |
| Backup/restore nyata | Manual pending |
| Pemasangan skill native | Manual pending |
| Deployment/service/tunnel/DNS | Belum diterapkan; perlu persetujuan |

Jangan mengubah status menjadi lulus tanpa hasil pengujian. Jika source akhir belum memenuhi kontrak, laporkan fitur sebagai belum tersedia dan revisi panduan sebelum rilis.

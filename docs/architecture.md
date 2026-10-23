# Arsitektur

## Kontrak dan bukti

Dokumen ini mengikuti source CLI, `internal/memory`, `internal/access`, `internal/server`, dan JavaScript dashboard yang telah ditinjau secara statis. Keberadaan kode bukan bukti uji runtime. HTTP, autentikasi, dashboard, MCP, dan deployment tetap perlu diverifikasi melalui acceptance di [integrations.md](integrations.md).

```text
Browser admin ── sesi + CSRF ── dashboard
                                   │
OpenCode / Hermes ── bearer ── /mcp ├── otorisasi proyek + izin
                                   ├── catatan Markdown berversi
                                   └── SQLite autentikasi / audit
```

Satu proses `memona serve` melayani dashboard dan MCP. Tidak ada worker yang mengeksekusi skrip skill. Setiap akses agen harus melewati autentikasi, pemeriksaan izin, dan pemeriksaan proyek pada server, termasuk akses langsung dengan ID.

## Penyimpanan pengetahuan

Dengan `data_dir: "./data"`, catatan berada di:

```text
data/
  notes/
    <id>/
      000001.md
      000002.md
  ... file autentikasi, audit, dan lock milik aplikasi ...
```

Frontmatter berupa objek **JSON yang sah sebagai YAML**, diapit delimiter `---`, diikuti body Markdown. Metadata mencakup ID, proyek, jenis, judul, konten, status, sumber, penulis, nomor revisi, waktu, penanda arsip, serta opsional `is_project_summary`, `related_to` dan `supersedes`. Source menyimpan konten juga pada metadata untuk mendeteksi body yang terpotong/tidak cocok. Jangan mengedit salah satu bagian secara manual. Field baru kosong tidak ditulis agar catatan lama tetap terbaca; rollback binary lama setelah metadata baru ditulis mempunyai batas pada [kontrak relasi](memory-relations.md).

- Revisi baru selalu file baru; revisi lama tidak ditimpa. Nomor revisi terakhir yang valid menentukan keadaan terbaru (*latest wins*), bukan waktu modifikasi filesystem.
- Perubahan memakai `expected_revision`: baca versi terbaru sebelum mutasi. Versi yang sudah tertinggal menghasilkan konflik; baca ulang dan tinjau perubahan, jangan retry buta dengan menimpa konten.
- Respons catatan memakai `revision`; input mutasi MCP dan publish/archive UI memakai tepat `expected_revision`. `revise_memory` memerlukan seluruh field pengganti, bukan patch parsial. Rincian argumen terdapat di [integrasi](integrations.md).
- `flock` eksklusif dipertahankan selama proses/store hidup. Proses kedua terhadap direktori data yang sama harus gagal, bukan menjadi writer paralel. Jangan menghapus file lock untuk memaksa akses.
- Scope proyek adalah slug logis, bukan path bebas dari agen. Lokasi filesystem dibatasi direktori data proyek yang dikonfigurasi, bukan home atau seluruh server.
- Source storage menolak symlink/traversal dan memakai akses relatif descriptor. Filesystem lokal diperlukan; network filesystem/multi-instance writer tidak diasumsikan aman.
- Penulisan diselesaikan dan di-fsync di `data/.staging`, kemudian dipindahkan atomik menggunakan `renameat2(RENAME_NOREPLACE)`; catatan baru juga dipublikasikan sebagai direktori lengkap. Linux/filesystem yang mendukung operasi ini diperlukan. Sisa staging akibat interupsi dipertahankan dan tidak masuk pembacaan catatan aktif; tidak ada cleanup otomatis.
- Revisi committed yang rusak dianggap kesalahan, bukan diam-diam dilewati. Pertahankan berkas untuk diagnosis; jangan hapus revisi secara otomatis.
- Proyek dan jenis catatan tidak dapat diganti lewat revisi. Buat catatan terpisah jika lingkupnya berbeda.

SQLite `data/memona.db` menyimpan admin, API key, sesi, dan audit; tidak menyimpan indeks pencarian ataupun salinan pengetahuan. Backup harus tetap mencakup **seluruh** direktori data karena Markdown saja tidak memulihkan key, admin, dan audit.

Empat mutation tool MCP mendukung `operation_id` opsional. Receipt storage-only `_operation` berisi hash scope/payload ditulis atomik dalam revisi yang sama, tanpa raw ID atau tabel transaksi lintas Markdown/SQLite. Indeks receipt in-memory dibangun ulang dari histori saat startup; retry key/tool/ID/payload sama mengembalikan revisi commit awal, bukan latest. Receipt tidak diwariskan ke revisi berikutnya dan tidak muncul pada JSON publik. Retensi mengikuti histori tanpa TTL; audit tetap per request, bukan exactly-once. Lihat [idempotensi dan kompatibilitas rollback](idempotency.md).

## Pencarian dan konteks

Diagnostik terpisah: `/healthz` liveness minimal tanpa disk/isi, `/api/storage-health` hanya sesi admin. Inspector read-only berbagi validasi canonical/history tetapi mengumpulkan hitungan temuan dan membatasi scan (20.000 entri/256 MiB/timeout kooperatif 5 detik), tidak rebuild receipt atau memodifikasi catatan. CLI `inspect -data-dir` membuka direktori/lock yang sudah ada dengan shared flock, bukan memanggil writer Open; bisa mendiagnosis histori yang membuat startup gagal. Tidak membuka SQLite. Rincian [storage health](storage-health.md).

Storage memvalidasi catatan saat dibuka. Setiap pencarian memanggil `notes.List()` untuk memindai catatan; **belum ada indeks pencarian**. Pencocokan menggunakan `strings.ToLower` dan `strings.Contains` pada judul, konten, dan sumber, bukan Unicode full case folding, embedding, vector database, semantic ranking, atau SQLite FTS.

`search_memory` hanya mengembalikan ringkasan catatan terbaru nonarsip yang diizinkan; hasil diurutkan berdasarkan waktu pembaruan. `read_memory` membaca versi terbaru berdasarkan ID, termasuk catatan arsip jika scope mengizinkan. Riwayat lengkap tersedia di detail UI, bukan argumen versi historis MCP.

`get_context` mengambil proyek yang diminta dan `global` hanya jika key diizinkan; `global` tidak otomatis melewati scope. Tanpa `project`, permintaan diarahkan ke `global`. Ringkasan proyek knowledge diprioritaskan, lalu ringkasan global yang diizinkan, kemudian terbaru. Skill, arsip, dan catatan yang secara eksplisit digantikan sumber nonarsip tidak masuk konteks. Filter `kind` berlaku; ringkasan melewati filter query. Limit 1–12/default 12 dengan anggaran 24.000 byte seluruh hasil JSON terstruktur, termasuk notes/metadata/referensi, selection_reasons dan guidance, bukan envelope protokol. Ranking leksikal title/content/source 6/2/1 berlaku setelah prioritas overview; mixed context maksimal dua handoff (kind=handoff opt-out kuota). Label is_testing dikecualikan default dengan include_testing=true untuk opt-in; search/read tetap tersedia sesuai izin.

Backlink dihitung dari revisi terbaru Markdown, tanpa menulis ulang target. Relasi hanya satu proyek, maksimal 20 ID per jenis; supersedes tanpa siklus, termasuk melewati arsip. Perubahan relasi membuat revisi baru dengan optimistic concurrency. Read/skill/context menyertakan `connections`, search memberi peringatan `superseded_by` dan `relations_truncated`; `get_memory_relations` memuat referensi saja. Tidak ada auto-traversal atau auto-link dari teks. Penetapan metadata melalui MCP perlu read+write; keluaran difilter scope sebelum judul/referensi diungkap.

Biaya pemindaian tumbuh dengan jumlah catatan/revisi. Benchmark, batas volume praktis, kecukupan pencocokan lowercase Unicode, dan strategi indeks lanjutan masih perlu pengukuran, bukan angka performa yang diasumsikan.

## Alur pengetahuan dan skill

Jenis catatan pada source storage: `fact`, `decision`, `knowledge`, `handoff`, `skill`. Status yang dikenali mencakup `unverified`, `verified`, `draft`, dan `active` khusus skill. Label verified memerlukan sumber, tetapi tidak membuktikan kebenaran isi secara otomatis; referensi `source` tidak dibuka atau dieksekusi oleh storage.

Skill mengikuti `draft → review admin → publish/active`. Revisi atas skill aktif harus kembali menjadi draft sehingga perlu review ulang. Agen tidak mempunyai izin publish. Arsip menambah revisi penanda, tidak menghancurkan berkas. Admin dapat memulihkan arsip melalui dashboard; pemulihan menambah revisi, mempertahankan isi dan riwayat. Tidak tersedia tool MCP untuk pemulihan atau penghapusan permanen.

Skill disimpan sebagai catatan `kind: "skill"` di `data/notes/<id>/`, bukan direktori `data/skills`. `get_skill` menerima `id` dan mengembalikan objek catatan dengan teks prosedur jika statusnya `active` dan tidak diarsipkan. Hasil ini tetap data; tidak ada packaging native, registry skill otomatis, auto-install, eksekusi skrip, atau endpoint katalog HTTP. Adaptasi manual ke skill native berada di luar aplikasi.

Dashboard adalah pembaca/filter catatan dan riwayat, dengan aksi publish skill, archive, pemulihan arsip, serta revisi metadata ringkasan/relasi lewat form detail. Tidak ada editor konten atau formulir membuat catatan; gunakan MCP untuk create/revise isi. Pengelolaan API key dinamis (buat, daftar, revoke, izin/proyek/kedaluwarsa) tersedia melalui UI.

## Dependensi

Target toolchain Go 1.26.8, official `github.com/modelcontextprotocol/go-sdk` v1.8.0, `github.com/yuin/goldmark` v1.8.6, dan `modernc.org/sqlite` v1.61.0. Versi modul dicatat di `go.mod` dan checksum di `go.sum`; jangan upgrade massal untuk menjalankan contoh. Direktif Go 1.26.0 pada manifest adalah baseline, bukan bukti penggunaan patch 1.26.8 saat build.

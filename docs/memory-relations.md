# Ringkasan proyek, relasi dan backlink

## Kontrak

Markdown tetap sumber utama; tidak ada database graph atau indeks independen baru. Frontmatter revisi dapat memiliki `is_project_summary: true`, `related_to: [ID]` dan `supersedes: [ID]`. Field kosong tidak ditulis. Catatan lama tanpa field ini tetap dibaca tanpa ditulis ulang.

1. **Ringkasan proyek:** catatan `knowledge`, isi maksimal 8000 byte, satu nonarsip per proyek. Isi sebaiknya tujuan, keputusan berlaku, referensi backlog dan langkah berikut. Penandaan eksplisit, bukan tebakan judul. Untuk mengganti ringkasan, hapus penanda lama atau arsipkan dahulu. Restore ringkasan ditolak bila proyek sudah mempunyai ringkasan nonarsip lain.
2. **Related-to:** referensi ke catatan penjelas, bukan penggantian. Arahnya eksplisit; backlink menunjukkan arah balik tanpa membuat revisi pada target.
3. **Supersedes:** `A.supersedes=[B.id]` berarti A menggantikan B secara utuh. Jangan pakai untuk penggantian sebagian; pakai related-to dan penjelasan isi. Tidak ada inferensi berdasarkan tanggal. Diri sendiri, target hilang, beda proyek, ID ganda lintas daftar dan siklus supersedes ditolak. Maks. 20 ID per jenis. Siklus terkait diperbolehkan. Siklus supersedes tetap diperiksa melewati arsip agar restore tidak menghidupkan siklus.
4. **Backlink:** diturunkan dari revisi terbaru sumber nonarsip, bukan file histori. Target arsip boleh dirujuk dan diberi penanda. Empat kelompok: `related_to`, `supersedes`, `referenced_by`, `superseded_by`, maksimal 20 referensi per kelompok dengan `truncated` eksplisit. Urutan deterministik berdasarkan ID; bukan skor relevansi atau jumlah total.

Relasi sengaja dibatasi dalam satu proyek. Scope read diperiksa sebelum referensi, judul, atau truncation dikembalikan. Catatan global tidak menjadi pengecualian. Tidak ada auto-link dari teks/URL, traversal rekursif, depends_on, embedding, visualisasi jaringan, maupun sinkronisasi dua arah dengan docs source.

## MCP dan dashboard

- `save_memory`/`save_handoff`/`revise_memory` mengenali tiga field opsional. Field yang dipasok memerlukan read+write; keenam field inti tetap wajib. Ringkasan tidak cocok handoff/skill. Saat revisi, hilang/null berarti pertahankan metadata; `false` menghapus penanda, `[]` mengosongkan relasi. Untuk kepastian gunakan hilang atau nilai eksplisit, bukan null.
- `get_context`: ringkasan proyek diminta, ringkasan global jika diizinkan, lalu ranking leksikal, catatan tahan lama sebelum handoff dan recency. Mixed context maksimal dua handoff, kind=handoff mengabaikan kuota kategori itu. Label is_testing dikecualikan default (include_testing=true opt-in), bukan pembatas akses search/read. Filter kind berlaku, ringkasan melewati filter query. Limit 1–12/default 12, anggaran seluruh hasil JSON terstruktur termasuk alasan pemilihan/guidance 24.000 byte (bukan seluruh envelope protokol). Referensi boleh dikosongkan dengan connections.truncated=true jika perlu mempertahankan isi catatan dalam anggaran.
- Catatan yang memiliki pengganti nonarsip dikecualikan dari context. Search masih mengembalikannya dengan `superseded_by`; read/skill mengembalikan `connections`. Arsip pengganti menghapus peringatannya dan target boleh kembali masuk context; restore mengaktifkan hubungan lagi. Status verifikasi/publikasi bukan penentu relasi; keputusan menggantikan harus dinyatakan secara sadar, termasuk bila sumber merupakan draft skill.
- `get_memory_relations({"id":"ID_NYATA"})` memerlukan read dan mengembalikan empat kelompok referensi, tidak isi target. Sepuluh tool tersedia setelah restart/reconnect. Jangan menganggap tool baru terpasang pada server lama.
- Dashboard mengurutkan ringkasan lebih dahulu, menandai catatan digantikan, dan menampilkan referensi yang dapat dibuka. Detail menyediakan form metadata via POST `/api/notes/<id>/connections` (sesi admin, CSRF, expected_revision). Bukan editor isi atau hard delete. Isi disalin dari revisi server saat ini, konflik tidak diulang otomatis. Metadata skill aktif direvisi menjadi draft.
- Setiap perubahan metadata membuat revisi baru. Histori tetap menyimpan ID dan penanda pada saat itu, sementara judul referensi yang ditampilkan pada hubungan berasal dari versi terbaru target. Backlink tidak menulis ulang target.

## Kompatibilitas dan aktivasi

Build tidak mengubah data aktif atau menjalankan migrasi. Klien lama boleh mengabaikan metadata baru; revisi tanpa metadata mempertahankannya. Respons revisi key write-only tidak memuat metadata yang dipertahankan agar ID/penanda yang belum boleh dibaca tidak bocor.

**Rollback binary lama tidak aman sesudah metadata baru ditulis.** Reader lama memverifikasi canonical encoding dan tidak mengenali field baru; bisa menolak catatan yang sudah mempunyai relasi/ringkasan. Jangan menghapus field atau mengedit revisi untuk memaksanya. Sebelum aktivasi, lakukan backup konsisten saat aplikasi berhenti mengikuti [operasi](operations.md). Pemulihan binary/data lama harus memakai pasangan backup yang sesuai ke direktori terpisah; alternatif lebih aman adalah perbaikan maju dengan build yang mengenali format ini. Jangan overwrite data aktif.

Aktivasi/restart serta penetapan relasi pada catatan nyata adalah langkah terpisah; implementasi tidak menandai atau menautkan catatan pengguna secara otomatis. Evaluasi kebutuhan depends_on/visualisasi setelah pemakaian nyata, bukan menambah graph karena tersedia.

## Acceptance tambahan (manual pending)

- Dua harness menemukan sepuluh tool dan skema opsional; uji save/read/revise ringkasan serta relasi sintetis.
- Browser kedua tema dan layar kecil: badge, form, error, konfirmasi supersedes, buka target, fokus setelah navigasi/tutup, keyboard serta histori metadata.
- Uji backup/restore relasi dan rollback pada direktori terisolasi. Test otomatis bukan bukti acceptance browser/harness/operasional.

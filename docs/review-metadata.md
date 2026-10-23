# Metadata waktu verifikasi dan kebutuhan review (QUAL-02)

Metadata ini berlaku untuk catatan `fact`, `decision`, `knowledge`, dan `handoff`, termasuk ringkasan proyek (`knowledge`). Tidak ada migrasi: revisi Markdown lama tetap kanonis dan dapat dibaca tanpa penambahan field.

## Kontrak storage dan JSON

`memory.Note` menambahkan:

- `VerifiedAt string` → `verified_at,omitempty`.
- `NeedsReview bool` → `needs_review,omitempty`.

`memory.Input` menambahkan:

- `VerifiedAt *string` → `verified_at,omitempty`.
- `NeedsReview *bool` → `needs_review,omitempty`.

Field pointer membedakan omission dari clear eksplisit. JSON `null` pada input setara dengan omission, bukan clear. Pada hasil dan Markdown tersimpan, string kosong dan `false` tidak dituliskan.

| Input | Create | Revise |
| --- | --- | --- |
| `verified_at` dihilangkan / `null` | Tidak menambah waktu | Mempertahankan waktu selama status tetap `verified`; menghapus waktu warisan saat status menjadi selain `verified` |
| `verified_at: ""` | Tidak menambah waktu | Menghapus waktu |
| `verified_at` berisi timestamp | Menetapkan waktu jika valid | Mengganti waktu jika valid |
| `needs_review` dihilangkan / `null` | Tidak menambah flag | Mempertahankan flag |
| `needs_review: false` | Tidak menambah flag | Menghapus flag |
| `needs_review: true` | Menetapkan flag | Menetapkan flag |

Timestamp nonkosong mensyaratkan `status: "verified"` dan `source` berisi referensi bukti nonblank, sesuai aturan status yang sudah ada. Bentuk yang diterima harus persis hasil roundtrip UTC `time.RFC3339Nano`, misalnya `2026-01-02T03:04:05Z` atau `2026-01-02T03:04:05.123456789Z`. Offset `+00:00`, zona selain `Z`, pecahan dengan nol akhir (`.100Z`), presisi melebihi nanodetik, dan whitespace ditolak; server tidak menormalisasi input diam-diam.

Waktu adalah **deklarasi pemanggil**, bukan bukti server telah memverifikasi isi/sumber. Server tidak membuka sumber atau memeriksa kebenaran klaim. Timestamp masa depan tidak ditolak: validasi data dan histori tidak bergantung pada jam saat ini. Waktu juga tidak dibatasi oleh `created_at`/`updated_at`. Mengubah status kembali menjadi `verified` tidak menghidupkan waktu yang pernah dihapus. `needs_review: true` boleh berdampingan dengan status `verified`; flag bukan status baru atau perubahan izin.

Omission tetap mempertahankan timestamp ketika judul, isi, atau sumber berubah tetapi status tetap `verified`. Karena itu waktu verifikasi terakhir yang dideklarasikan dapat mendahului perubahan isi; penyimpanan tidak menyatakan isi terbaru telah diperiksa ulang. Penulis perlu meninjau ulang dan mengganti/menghapus waktu secara eksplisit bila sesuai. `needs_review` juga sepenuhnya dikendalikan penulis, tidak otomatis diaktifkan atau dibersihkan oleh perubahan isi maupun timestamp.

Untuk `kind: "skill"`, kedua field harus dihilangkan atau `null`. Semua metadata review eksplisit pada skill ditolak, termasuk `verified_at: ""` dan `needs_review: false`. Metadata skill dan alur draft/publish tetap terpisah.

## Revisi, lifecycle, dan idempotensi

API storage yang memakai kontrak ini: `Create`, `CreateContext`, `Revise`, `ReviseContext`, `Execute`, dan `ExecuteContext`. Revisi tetap memerlukan seluruh field pengganti wajib, serta `expected_revision` pada adapter pemanggil; omission metadata opsional mempertahankan nilai seperti tabel di atas. Project/kind tidak dapat diubah, termasuk dengan mencoba mengirim clear metadata bersamaan.

`Archive`/`ArchiveContext` dan `Restore`/`RestoreContext` mempertahankan metadata. Histori menolak perubahan metadata bersamaan dengan archive/restore. `Publish`/`PublishContext` tetap khusus skill tanpa metadata review. Input eksplisit kedua field pada `Execute`/`ExecuteContext` dengan tool `archive_memory` ditolak: gunakan revisi terpisah untuk mengubah metadata, bukan lifecycle mutation. Pemanggil archive mengirim `memory.Input{}` seperti sebelumnya.

Fingerprint operation menambahkan pointer `VerifiedAt` lalu `NeedsReview` sesudah `SkillMetadata`, keduanya `omitempty`. Payload lama tanpa field baru tetap byte-identik dan receipt versi 1 tetap berlaku; omission berbeda dari string kosong/`false` eksplisit. Replay mengembalikan revisi asli dengan metadata saat operasi pertama, bukan revisi terbaru. Pembatalan sebelum staging atau validasi gagal tidak membuat revisi atau memesan operation ID.

Tidak ada inferensi konflik, penggantian, atau freshness dari timestamp/flag. Penggantian tetap hanya dinyatakan melalui `supersedes` eksplisit. Metadata ini tidak mengubah auth, scope, permission, sumber bukti, atau data SQLite.

## Dashboard dan retrieval

Editor admin menyediakan flag perlu review dan waktu verifikasi UTC, tombol waktu sekarang serta clear eksplisit. Status selain verified menonaktifkan waktu dan membersihkan nilai form sebelumnya; validasi server tetap authoritative. Daftar memiliki badge dan filter lokal “Hanya perlu ditinjau” pada maksimal 500 hasil yang sudah dimuat. Detail dan diff metadata menampilkan deklarasi ini tanpa menganggap verified menyelesaikan review.

MCP save/revise menerima field melalui skema Input; search mengembalikan verified_at/needs_review, read/context mempertahankannya, dan selection_reasons context menambahkan WARNING untuk perlu review. Flag tidak menghilangkan catatan dari retrieval, tidak memperluas scope, dan tidak memilih salah satu klaim bertentangan. Tidak ada jadwal kedaluwarsa atau deteksi konflik semantik otomatis. Sumber, status dan jenis decision/fact/knowledge/handoff tetap cara membedakan maksud penulis.

Binary lama dapat menolak revisi immutable yang mengandung metadata baru; membersihkan revisi terbaru tidak menghilangkan field dari histori. Rollback memerlukan binary/data kompatibel, bukan mengedit atau menghapus histori.

## Verifikasi otomatis

`internal/memory/review_metadata_test.go` mencakup kompatibilitas byte Markdown lama, omission/null/clear JSON, validasi UTC kanonis dan kind/status/source, ringkasan proyek, isolasi salinan, create/revise langsung dan context, preserve/clear/perubahan status, lifecycle, fingerprint dan replay receipt lama/baru setelah restart, serta pembatalan dan kegagalan validasi tanpa pemesanan receipt. Test storage bukan bukti acceptance UI, transport MCP/HTTP, atau deployment.

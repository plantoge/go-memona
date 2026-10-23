# Metadata eksplisit catatan pengujian

## Kontrak storage

`memory.Note.IsTesting` adalah `bool` dengan JSON `is_testing,omitempty`.
`memory.Input.IsTesting` adalah `*bool` dengan JSON `is_testing,omitempty`:

| Input | Create | Revise |
| --- | --- | --- |
| Field tidak dikirim (`nil`) | `false` | Pertahankan nilai revisi sebelumnya |
| `true` | Tandai sebagai catatan pengujian/contoh | Tetapkan `true` |
| `false` | Catatan biasa | Hapus penanda |

Penanda ini eksplisit, bukan hasil pencocokan judul, isi, proyek, status, atau
sumber. Tidak ada migrasi atau pelabelan otomatis catatan lama. Semua jenis
catatan tetap menggunakan validasi status yang sama; penanda tidak memberikan
izin tambahan dan tidak melemahkan autentikasi atau otorisasi.

`noteInput` memetakan nilai tersimpan menjadi pointer eksplisit. Seluruh jalur
create/revise (langsung, context, dan Execute/ExecuteContext) memakai penerapan
metadata yang sama. Publish, archive, dan restore mempertahankan penanda.
Validasi transisi histori menolak perubahan penanda bersamaan dengan ketiga
transisi tersebut. Mengedit skill aktif tetap mengembalikannya menjadi draft.

## Format immutable dan kompatibilitas

Nilai `false` tidak ditulis dalam JSON publik atau frontmatter canonical.
Revisi lama yang tidak mempunyai field ini tetap dapat dibaca dan di-encode
kembali dengan byte yang sama. Nilai `true` ditulis dalam frontmatter revisi
baru; revisi sebelumnya tidak ditulis ulang. Representasi redundant
`"is_testing":false` dalam berkas immutable bukan bentuk canonical dan ditolak,
sama seperti flag opsional canonical lainnya.

Binary lama yang belum mengenal field ini **tidak aman untuk rollback** setelah
revisi dengan `is_testing:true` dibuat: pemeriksaan canonical lama dapat menolak
berkas tersebut. Jangan menghapus metadata atau mengedit revisi lama untuk
memaksakan rollback. Gunakan binary yang mendukung format baru, atau pulihkan
backup konsisten sebelum perubahan ke direktori terpisah sesuai prosedur dan
persetujuan operasi. Revisi baru yang menghapus flag tidak menghapus histori
lama yang masih memerlukannya.

## Receipt idempotensi

Versi receipt tetap 1. Fingerprint mempertahankan nama/urutan/encoding seluruh
field payload lama, termasuk perbedaan null dan array kosong pada relasi.
`IsTesting` ditambahkan di akhir payload fingerprint dan dihilangkan hanya
ketika pointer `nil`. Karena itu:

- Request lama tanpa `is_testing` menghasilkan fingerprint yang identik dan
  receipt lama tetap mengembalikan revisi asli, bukan catatan latest.
- `nil`, `false`, dan `true` adalah tiga payload berbeda. Mengganti atau
  menambahkan field pada retry dengan operation ID yang sama ditolak sebagai
  `ErrOperationConflict`, bahkan jika hasil akhir create kebetulan sama.
- Tidak ada fallback yang mengabaikan field baru saat memeriksa receipt lama.
- Receipt tetap berada dalam revisi immutable; restart membangun ulang indeks
  dari revisi tersebut tanpa mengubah berkas.
- Cancellation sebelum staging tidak mengubah metadata, tidak menambah revisi,
  dan tidak mencadangkan operation ID. Batas commit atomik tidak berubah.

## Batas integrasi

Dokumen ini menjelaskan implementasi `internal/memory`. Filter MCP
`include_testing`, visibilitas hasil, panduan agen, dan checkbox UI adalah
lapisan terpisah; penambahan field storage sendiri tidak otomatis menerapkan
filter atau membuktikan acceptance browser/lintas harness. Read/history storage
mengembalikan metadata sesuai revisi yang dibaca, tanpa menyembunyikan catatan.

## Bukti pengujian pengembangan

Test sintetis ada di `internal/memory/testing_metadata_test.go`, mencakup byte
canonical lama, presence JSON, create/revise preserve/clear, persistensi restart,
histori, publish/archive/restore, penolakan transisi yang mengubah metadata,
fingerprint receipt lama, replay asli setelah revisi berikutnya dan restart,
konflik omit/false/true, serta cancellation dan retry. Test storage lain tetap
dijalankan untuk regresi.

Perintah offline yang dijalankan pada 22 Oktober 2026:

```bash
env GOPROXY=off GOSUMDB=off make test GO=/usr/local/go/bin/go

env GOPROXY=off GOSUMDB=off \
  GOCACHE="$PWD/.cache/build" GOMODCACHE="$PWD/.cache/mod" \
  GOPATH="$PWD/.cache/go" GOTMPDIR="$PWD/.cache/tmp" \
  /usr/local/go/bin/go test -race ./internal/memory

env GOPROXY=off GOSUMDB=off \
  GOCACHE="$PWD/.cache/build" GOMODCACHE="$PWD/.cache/mod" \
  GOPATH="$PWD/.cache/go" GOTMPDIR="$PWD/.cache/tmp" \
  /usr/local/go/bin/go vet ./internal/memory
```

Ketiganya lulus. Cache mengikuti Makefile; unduhan modul dinonaktifkan. Tidak
ada deployment, instalasi, perubahan data aktif, auth, dependency, atau commit.
Direktori kerja tidak mempunyai repository Git sehingga `git status`/`git diff`
tidak tersedia. Acceptance manual dan rollback operasional belum dijalankan.

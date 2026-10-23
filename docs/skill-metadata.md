# Metadata skill terstruktur

## Kontrak penyimpanan

`memory.Note.SkillMetadata` dan `memory.Input.SkillMetadata` bertipe
`*memory.SkillMetadata`, dengan JSON `skill_metadata,omitempty`. Metadata ini
opsional dan hanya boleh disertakan pada `kind: "skill"`, termasuk objek kosong.
Catatan skill lama tanpa metadata tetap valid. Tidak ada migrasi atau penulisan
ulang revisi lama.

```json
{
  "kind": "skill",
  "status": "draft",
  "skill_metadata": {
    "trigger": "Gunakan untuk pemeriksaan rilis lokal",
    "version": "1.0",
    "compatibility": ["Go 1.26", "Linux"],
    "test_evidence": "Unit test lokal; rujukan: release-check_test.go"
  }
}
```

Contoh ini hanya cuplikan metadata, bukan seluruh input mutasi. Field wajib input
seperti `project`, `title`, `content`, dan `source` tetap mengikuti kontrak lama.

| Field Go | Field JSON | Batas |
| --- | --- | --- |
| `Trigger string` | `trigger` | 1000 byte |
| `Version string` | `version` | 100 byte |
| `Compatibility []string` | `compatibility` | Maksimum 8 string |
| `TestEvidence string` | `test_evidence` | 2000 byte |

Jumlah byte seluruh nilai teks, termasuk semua string compatibility, maksimum
4000 byte. Semua nilai harus UTF-8 valid dan tidak mengandung karakter kontrol,
termasuk NUL, tab, newline, dan DEL. Batas adalah byte UTF-8, bukan jumlah karakter.
Field kosong diperbolehkan; tidak ada parsing versi atau pemeriksaan kemampuan
runtime. Semua field metadata memakai `omitempty`.

## Create, revise, dan clear

- Create dengan field tidak ada atau `null` (`nil`) berarti tidak ada metadata.
- Revise dengan field tidak ada atau `null` berarti mempertahankan metadata lama.
- Objek nonkosong mengganti **seluruh** metadata; bukan patch per subfield.
- Objek kosong `{}` secara eksplisit menghapus metadata pada revise. Objek dengan
  seluruh string kosong dan compatibility kosong juga dinormalisasi ke `nil` pada
  Note. Create dengan objek kosong menghasilkan Note tanpa field metadata.
- Pointer dan backing array compatibility disalin ketika metadata diterapkan atau
  dipetakan dari Note ke Input, sehingga perubahan objek pemanggil tidak mengubah
  metadata Note atau revisi tersimpan.

Metadata deskriptif ini tidak memberikan izin, bukan bukti terverifikasi otomatis,
bukan paket skill native, dan tidak memuat atau mengeksekusi kode. Bukti test yang
ditulis pengguna tetap klaim yang perlu diperiksa. Publish tetap mengikuti alur
admin yang ada, tanpa syarat publish baru. Edit skill active tetap kembali ke
draft; omission metadata mempertahankannya. Publish, archive, dan restore harus
mempertahankan seluruh metadata, dan edit catatan archived tetap ditolak.

## Kanonisasi, retry, dan kompatibilitas

Revisi Markdown tetap immutable. Field metadata nil dihilangkan dari encoding
kanonik; revisi lama, termasuk yang sudah menggunakan `is_testing`, menghasilkan
byte yang sama ketika dibaca dan dienkode kembali. Objek metadata kosong hanya
merupakan intent input: pada Note dinormalisasi menjadi nil dan tidak ditulis
sebagai `{}`. JSON tersimpan dengan field kosong redundan, null, atau objek kosong
bukan encoding kanonik dan ditolak sebagai korup.

Fingerprint receipt operasi menambahkan pointer `SkillMetadata` dengan
`omitempty` **setelah** field opsional `IsTesting`; urutan dan encoding payload
lama tidak berubah bila metadata tidak ada. Fingerprint memakai intent Input asli,
bukan Note yang sudah dinormalisasi: omission dan objek kosong berbeda. Mengulang
operation ID dengan payload sama mengembalikan revisi asli meskipun ada revisi
berikutnya; mengganti metadata atau mengubah omission menjadi clear menghasilkan
konflik operasi. Riwayat dan receipt direkonstruksi saat store dibuka kembali.

## Rollback

Binary lama tetap dapat membaca data yang belum pernah menyimpan metadata
nonkosong. Namun **binary lama tidak dapat membaca revisi baru yang menyimpan
`skill_metadata` nonkosong**: encoding kanonik lama tidak mengenal field itu dan
pemeriksaan integritas gagal. Menghapus metadata pada revisi terbaru tidak membuat
rollback aman, sebab revisi immutable sebelumnya masih memuat field tersebut.
Receipt dari input clear `{}` juga memakai fingerprint baru dan tidak dapat
dianggap kompatibel dengan retry dari binary lama.

Untuk rollback yang konsisten, hentikan aplikasi sesuai prosedur operasional dan
pulihkan backup sebelum penggunaan metadata ke direktori data terpisah. Jangan
mengedit/menghapus revisi lama, menimpa direktori aktif, atau mengabaikan pemeriksaan
integritas. Perubahan ini tidak melakukan deployment, migrasi, backup, atau
pemulihan data aktif.

## Cakupan verifikasi

Test penyimpanan berada di `internal/memory/skill_metadata_test.go`: encoding lama,
receipt lama dengan `is_testing` tidak ada/false/true, batas validasi, isolasi copy,
preserve/replace/clear, restart, replay receipt, publish/archive/restore, serta
active kembali ke draft. Integrasi MCP dan form UI ditangani terpisah; test storage
ini bukan bukti acceptance browser atau deployment.

# Matriks acceptance yang dapat diulang — DEV02

## Status dan batas bukti

Matriks ini adalah **rencana acceptance, bukan sertifikat kelulusan**. Browser nyata, dua harness OpenCode–Hermes, aktivasi service hidup, dan reboot tetap **pending**. Review source atau test lokal tidak membuktikan bahwa binary aktif/harness discovery/cache telah diperbarui. Hasil otomatis yang dilaporkan di [verification.md](verification.md) tetap milik run tersebut; jangan menyalinnya sebagai hasil manual baru.

Gunakan hanya instance/config/data sintetis yang terisolasi dan disetujui, bukan layanan/data/key pengguna yang aktif. Restart, revoke key, perubahan harness/global config, deployment, exposure dan reboot memerlukan persetujuan terpisah. Jangan menghapus data/lock/berkas rusak agar kasus tampak lulus. Panduan detail kontrak dan lima langkah lintas harness: [integrations.md](integrations.md).

## Identitas dan format evidence per run

Salin formulir ini ke catatan pengujian nonsecret. Kolom kosong berarti belum diuji, bukan pass.

| Field | Isi yang harus dicatat |
| --- | --- |
| Run ID, tanggal/waktu/timezone, pelaksana/reviewer | Penanda sintetis unik `MEMONA-ACCEPT-<run-id>` |
| Source dan artefak | Commit bila ada, dirty state, SHA-256 binary yang diuji, cara memastikan instance uji memakai artefak tersebut |
| Platform/toolchain | OS, Go, Node/npm bila build UI, versi systemd bila diuji |
| Client | Versi OpenCode dan Hermes, browser/OS, discovery tool/schema dari masing-masing harness |
| Isolasi | Alias instance, proyek sintetis `acceptance-demo` / `acceptance-other`, label key A/B/read-only tanpa token; konfigurasi nonsecret yang relevan |
| Kasus | ID kasus, prasyarat, langkah aktual, expected vs observed, waktu, exit/status code, pass/fail/blocked/pending |
| Evidence | Referensi log/screenshot/receipt teredaksi, ID catatan sintetis, revisi sebelum/sesudah, reviewer |
| Gap | Alasan blocked, risiko, pemilik tindak lanjut dan kebutuhan persetujuan |

Jangan melampirkan password, bearer, cookie, file environment, config runtime penuh, path privat, atau data pribadi. Screenshot tab key harus disunting. Respons teks agen tanpa receipt/tool result tidak cukup. `pass` memerlukan observed sesuai expected dan evidence; `fail` berarti sudah dijalankan tetapi tidak sesuai; `blocked` berarti tidak dapat dilakukan dengan alasan; `pending` berarti belum dilakukan. Jangan mengubah pending hanya karena dokumentasi ditulis.

## Gate lokal/source

| ID | Langkah yang dapat diulang | Expected / bukti wajib | Status awal |
| --- | --- | --- | --- |
| DEV01-A | Jalankan preflight dengan `--source-root` eksplisit pada checkout tepercaya; opsional hash artefak eksplisit. | JSON/exit code; label tetap dan hash, tanpa path/secret. Batas pemeriksaan dicatat di [version-control.md](version-control.md). | Pemeriksaan lokal 2026-10-22 exit 0, JSON `ok: true`; tanpa hash binary opsional. Bukan gate rilis lengkap. |
| DEV01-B | Review `.gitignore` dan calon baseline allowlist. | Tidak ada runtime/secret/data dalam staging; bila belum ada repo catat apa adanya. Tidak melakukan init/commit tanpa persetujuan. | Review terbatas tersedia; baseline Git belum dilakukan. |
| AUTO01 | Jalankan `make test`, `make race`, `make vet`, `make build` dengan cache proyek dan izin jaringan/dependency yang sesuai. | Command, versi, waktu, exit code dan ringkasan hasil untuk source/artefak yang sama. | Pending untuk run acceptance baru. |
| AUTO02 | Jalankan test Python preflight fixture sintetis. | Jumlah test, exit code, batas tanpa runtime read; bukan uji aplikasi hidup. | 2026-10-22: `python3 -B -m unittest discover -s scripts -p test_check_release.py -v`, 10 test, exit 0, `OK` (0,031 s). Bukan uji aplikasi hidup. |

## Lima langkah lintas OpenCode → UI → Hermes → OpenCode

Prasyarat: bootstrap instance uji disetujui, proyek/key sintetis scoped `acceptance-demo`, key berbeda tiap harness, discovery sepuluh tool cocok dengan build yang diuji. UI tidak memiliki editor create/revise.

| ID | Langkah | Expected / bukti wajib | Status |
| --- | --- | --- | --- |
| E2E01 | OpenCode `save_memory` decision unverified dengan penanda run dan isi pilihan A; buka UI. | Receipt tool berisi ID/revisi; UI menampilkan proyek, isi, penulis/status/revisi sama. | Pending — dua harness/browser belum diuji. |
| E2E02 | OpenCode read lalu revise dengan `expected_revision` terbaru dan enam field penuh; isi pilihan B; refresh UI/histori. | ID tetap, revisi naik, histori A immutable, detail terbaru B. | Pending. |
| E2E03 | Di sesi Hermes baru, `get_context`, search penanda, lalu read ID. | Hasil tool nyata memuat B pada scope yang benar; catat discovery/version dan revisi. | Pending. |
| E2E04 | Hermes `save_handoff` field lengkap, referensi ID dan langkah berikut; sesi OpenCode baru search/read. | Handoff yang sama terlihat di OpenCode dan UI, aktor sesuai key; tidak hanya salinan chat. | Pending. |
| E2E05 | Review histori/audit di UI, stop/start normal instance uji setelah persetujuan lalu baca dari dua harness. | Revisi/handoff bertahan; admin lama tetap; binary aktif teridentifikasi. | Pending — restart/aktivasi belum dilakukan. |

## Negatif, lifecycle dan browser

| ID | Setup / tindakan berulang | Expected dan evidence | Status |
| --- | --- | --- | --- |
| AUTH01 | Key scope demo mencoba context/search/read/revise/handoff/skill/relasi proyek other; hanya ID sintetis. | Ditolak atau hasil tidak membocorkan scope lain; receipt/status per tool. Project/kind tidak berubah. | Pending. |
| AUTH02 | Key read-only baca lalu save/revise/handoff/archive; key write tanpa archive mencoba archive. | Baca sesuai scope berhasil, mutasi/arsip tanpa izin ditolak di server. | Pending. |
| AUTH03 | Key uji valid dipakai, dicabut lewat UI, dipakai lagi dari klien lama dan baru. | Request berikut ditolak; key tidak tampil ulang, tidak bocor di bukti. | Pending. |
| REV01 | Dua klien membaca N, A revise N, B revise N. | B konflik; read ulang sebelum gabungan sadar. Simpan receipt N/N+1 dan konflik. | Pending. |
| LIFE01 | Archive dengan revisi benar; search/context aktif; admin restore via UI. | Tidak ada hard delete; archive receipt saja, histori tetap; restore menambah revisi. | Pending. |
| SKILL01 | Agen simpan draft; get_skill; admin publish; get dari dua harness; revise lagi. | Draft tidak diambil sebagai active, publish hanya admin; revisi kembali draft, perlu review ulang. UI tombol publish/error diperiksa. | Pending. |
| BOOT01 | Instance baru tanpa/kurang dari 16 karakter bootstrap; lalu bootstrap valid via mekanisme privat. | Startup gagal untuk input invalid; tidak ada default password. Jangan simpan nilai secret dalam evidence. | Pending. |
| BOOT02 | Restart instance uji dengan bootstrap berbeda setelah admin ada. | Password admin lama tidak tertimpa; operasi rutin tanpa bootstrap. | Pending. |
| LOCK01 | Proses kedua terhadap data sintetis yang sama; stop normal pertama lalu buka ulang. | Proses kedua ditolak; lock tidak dihapus paksa; revisi utuh saat reopen. | Pending. |
| WEB01 | Browser nyata login salah/benar, logout, akses sebelum login, kedaluwarsa sesi. | Otorisasi dan batas sesi 8 jam sesuai kontrak; catat elapsed time/metode, jangan mengklaim expiry dari review saja. | Pending — browser. |
| WEB02 | Uji cookie, mutasi tanpa CSRF dan origin/host salah pada instance uji. | HttpOnly/SameSite Strict, Secure untuk HTTPS publik; request invalid ditolak, origin valid tetap berfungsi. | Pending — browser/network terisolasi. |
| WEB03 | Catatan Markdown sintetis berbahaya; keyboard, layout responsif, konflik/error. | Tidak ada eksekusi JS; navigasi/error dapat dipahami; secret key hanya sekali. Screenshot teredaksi. | Pending — browser. |
| LIMIT01 | Beban terkendali pada instance uji: limiter login/MCP. | HTTP 429 sesuai kontrak global; tidak disamakan dengan kuota per-key, tanpa retry agresif/layanan aktif. | Pending. |
| REC01 | Stop instance uji, backup konsisten, restore ke direktori baru terpisah sesuai operations/recovery yang direview. | Admin/key/revoke/audit/notes/revisi/skill tetap; baca dua harness setelah restore. Keberhasilan arsip saja tidak cukup. | Pending — recovery manual. |

## Operasional dan exposure

| ID | Langkah | Expected / evidence | Status |
| --- | --- | --- | --- |
| OPS01-A | Review substitusi [service template](service-template.md), user nonroot, absolute paths, private bootstrap opsional, writable data dan sandbox. | Reviewer mencatat kompatibilitas unit/OS; tidak ada secret atau placeholder aktif. Tidak memasang unit saat review. | Template tersedia; review host pending. |
| OPS01-B | Setelah izin, jalankan salinan unit hanya pada VM/sandbox sintetis; uji SIGTERM, durasi stop, persistence/restart, filesystem sandbox. | Bukti user nonroot, write data berhasil/write source ditolak, exit/lock/shutdown tercatat. | Pending — live activation belum dilakukan. |
| OPS01-C | Reboot VM uji yang disetujui; cek boot/start dan read ulang. | Bukti boot ID/time, status service dan data sintetis utuh. Tidak disimpulkan dari unit syntax. | Pending — **reboot tidak diuji**. |
| NET01 | Setelah izin exposure/TLS, cek UI dan MCP melalui endpoint publik/proxy yang ditentukan. | HTTPS/cookie benar, MCP protokol bukan halaman HTML login, bearer tetap wajib. | Pending — tidak ada perubahan DNS/proxy/tunnel. |

## Keputusan rilis

Reviewer menandatangani hasil per kasus, bukan hanya skor agregat. Semua kasus pending/fail/blocked harus tetap terlihat dengan mitigasi dan pemilik. Jangan mengklaim “end-to-end lulus”, “production-ready”, “reboot tested”, atau “dua harness terverifikasi” tanpa evidence terkait. Aktivasi binary baru, restart layanan aktif, exposure dan publikasi Git adalah keputusan operator terpisah dari kelulusan unit test atau preflight.

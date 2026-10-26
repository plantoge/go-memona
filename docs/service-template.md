# Template service systemd — OPS01

**Status: contoh untuk review, bukan deployment.** Berkas [memona.service.example](../deploy/memona.service.example) belum dipasang, diaktifkan, atau dijalankan. Tidak ada hasil uji reboot, restart service aktif, atau live activation. Jangan menyalin template apa adanya ke konfigurasi sistem. Perubahan service, identitas OS, permission, listener, proxy, atau exposure memerlukan persetujuan terpisah.

## Kontrak dan placeholder

Review source `cmd/memona/main.go`: jalur service adalah `memona serve -config <file>`. SIGTERM/SIGINT meminta HTTP shutdown dengan anggaran 10 detik, kemudian penutupan auth/store melalui defer. Template memberi `TimeoutStopSec=30s`; ini bukan bukti shutdown pada host tujuan. Periksa request panjang, SQLite dan writer lock pada pengujian terisolasi. Jangan menghapus lock untuk memaksa restart.

| Placeholder | Keputusan operator sebelum penggunaan |
| --- | --- |
| `REPLACE_WITH_NONROOT_USER` / `REPLACE_WITH_NONROOT_GROUP` | Identitas service nonroot yang sudah disetujui, tanpa hak login/admin yang tidak perlu. Template tidak membuat akun. |
| `/ABSOLUTE/REPLACE_WITH_APPLICATION_DIRECTORY` | Direktori aplikasi absolut; executable `bin/memona` dan aset embed sudah dibuild/review. Identitas service hanya perlu baca/eksekusi, bukan menulis source/binary. |
| `/ABSOLUTE/REPLACE_WITH_PRIVATE_CONFIG_DIRECTORY/config.json` | File JSON privat yang sudah disiapkan dan dapat dibaca service. Gunakan `data_dir` absolut. Jangan simpan konfigurasi runtime di Git/bukti. |
| `/ABSOLUTE/REPLACE_WITH_DATA_DIRECTORY` | Direktori data terisolasi yang sudah ada, dimiliki/diizinkan secara terbatas untuk service. Harus sama dengan `data_dir`; mencakup SQLite, notes, staging dan lock, bukan sekadar berkas database. |
| `/ABSOLUTE/REPLACE_WITH_PRIVATE_BOOTSTRAP_DIRECTORY/bootstrap.env` | **Opsional, bootstrap saja**, lihat aturan berikut. |

Semua path aktif harus absolut, tanpa placeholder. Pilih path sederhana tanpa whitespace/karakter ekspansi unit. Jangan taruh aplikasi/config/data di home yang diblokir `ProtectHome=true`. Jangan memakai listener privileged; service tidak mempunyai capability. Konfigurasi network mengikuti [network.md](network.md): template tidak menyiapkan TLS/proxy maupun mengubah validasi origin/CSRF. Tidak ada `ExecStartPre` yang membuat/mengubah file.

## Bootstrap tanpa secret di template

`EnvironmentFile` sengaja dikomentari. Hanya boleh dibuka komentarnya untuk bootstrap ketika file privat **sudah ada**, sudah diperiksa operator, dan disetujui. File tersebut hanya membawa variabel bootstrap `MEMONA_ADMIN_PASSWORD`; nilainya tidak dicantumkan di unit, source, argumen proses, history, tiket, atau dokumentasi. File harus regular, bukan symlink, dibatasi aksesnya (misalnya mode 0600 milik administrator yang mengelolanya), serta bukan bagian repository. Evaluasi permission induk dan model akses systemd pada host tujuan; jangan memakai file yang world/group-readable. Jangan membaca file itu dengan preflight rilis.

Tidak memakai awalan `-` pada `EnvironmentFile`: bila sengaja diaktifkan tetapi file hilang, startup harus gagal, bukan diam-diam melewati bootstrap. Admin baru memerlukan password minimal 10 byte (disarankan acak dan lebih panjang); tidak ada default. Admin yang sudah ada tidak boleh ditimpa saat restart. Setelah bootstrap terverifikasi, hilangkan dependensi file bootstrap dari konfigurasi operasi rutin melalui perubahan yang disetujui. Jangan menghapus file/secret otomatis. Environment bukan secret store sempurna; akses privileged dan propagasi environment perlu review operator.

## Sandbox dan shutdown

- `User`/`Group` nonroot, `NoNewPrivileges=true`, capability kosong, `UMask=0077`.
- `ProtectSystem=strict` membuat filesystem read-only kecuali pengecualian eksplisit. `ReadWritePaths` hanya data; tidak memberikan permission Unix baru dan tidak membuat direktori.
- `ProtectHome`, `PrivateDevices`, proteksi kernel/control group dan pembatasan SUID mengurangi akses. `PrivateTmp` memberi area temporary privat; ini bukan janji bahwa hanya data yang dapat ditulis di seluruh namespace.
- `KillSignal=SIGTERM`, `KillMode=control-group`, dan timeout memberi jalur shutdown normal sebelum systemd dapat memaksa terminasi. `Restart=on-failure` dibatasi start limit agar kegagalan config/bootstrap tidak menjadi loop tanpa batas.
- Log dikirim ke journal. Batas retensi/akses log adalah keputusan operator; tidak diubah oleh template. Jangan memasukkan password/key/isi catatan ke bukti journal.

Kompatibilitas directive tergantung versi systemd/OS. Review kompatibilitas dan kebutuhan akses aplikasi, bukan menonaktifkan sandbox secara luas agar startup berhasil.

## Rencana verifikasi terisolasi — belum dijalankan

1. Catat OS/systemd, hash binary, revisi source dan reviewer. Review semua substitusi path, ownership, permission, network dan bootstrap tanpa melampirkan secret.
2. Pada VM/sandbox yang disetujui, buat instance/config/data **sintetis terpisah**. Validasi sintaks unit salinan dengan tooling systemd setempat setelah semua placeholder diganti. Validasi sintaks bukan bukti startup, sandbox, atau reboot.
3. Setelah persetujuan aktivasi instance uji: periksa startup nonroot, bootstrap awal/ulang, read/write di data, penolakan write ke aplikasi, dan akses UI/MCP di loopback uji. Jangan menyentuh service/data aktif.
4. Stop normal saat ada data sintetis; catat durasi, exit status, lock release dan read ulang setelah restart. Jalankan proses kedua untuk menguji penolakan lock tanpa menghapus lock. Catat hasil, bukan asumsi dari `TimeoutStopSec`.
5. Uji reboot hanya di VM yang boleh direboot dan dengan persetujuan eksplisit. Sampai ada bukti, status reboot tetap **pending**.

Catat setiap kasus di [acceptance-matrix.md](acceptance-matrix.md). Tidak ada perintah install/enable/start/restart service yang dijalankan oleh pekerjaan template ini. Pemulihan deployment yang kelak disetujui harus mempertimbangkan format storage dan backup konsisten; jangan menimpa data aktif dengan backup atau binary lama tanpa review kompatibilitas.

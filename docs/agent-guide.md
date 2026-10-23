# Panduan yang bisa dibaca harness

Panduan utama berada di `internal/server/agent-guide.md` dan ditanam ke executable agar tidak bergantung pada working directory.

## Tiga jalur discovery

1. **Instruksi MCP saat initialize:** aturan inti dikirim pada setiap koneksi, termasuk status yang benar dan arahan membaca panduan sebelum write.
2. **Tool `get_memory_guide`** dengan argumen `{}`: mengembalikan panduan Markdown lengkap. Tersedia untuk koneksi API key valid, tanpa membuka data proyek di luar scope. Key tidak perlu izin write untuk membaca dokumentasi umum.
3. **Resource `memona://guide`**: panduan sama untuk harness yang mendukung discovery dan pembacaan resource MCP.

Untuk build terbaru ada sebelas tool, termasuk `get_memory_relations` dan `find_duplicate_candidates`. Memuat panduan tidak melakukan save atau mengubah izin. Wrapper nama berbeda antarklien; gunakan discovery aktual, bukan nama prefiks tebakan. Panduan tidak bisa menjamin agen selalu patuh; verifikasi hasil tool dan instruksi persisten harness tetap diperlukan. Panduan terbaru juga menjelaskan ringkasan proyek, relasi eksplisit dan backlink.

## Contoh permintaan pengguna

> Baca get_memory_guide dari MCP Memona sebelum menyimpan. Ikuti pilihan kind/status, kemudian ambil konteks proyek yang relevan. Jangan mengaku menyimpan tanpa panggilan tool berhasil.

Di OpenCode resource dapat dibaca lewat list/read MCP resources. Hermes dapat menggunakan utility resource jika capability itu diekspos. Tool get_memory_guide adalah jalur sederhana yang tidak memerlukan utility resource.

## Kompatibilitas dengan proses lama

Executable baru tidak memperbarui proses server yang sedang berjalan. Hentikan Memona secara normal melalui Ctrl+C pada terminal pemilik dan jalankan kembali `./bin/memona serve -config config.json`. Setelah itu reconnect MCP (Hermes: /reload-mcp; OpenCode: /mcps). Tidak perlu mengubah key atau menghapus data. Restart harus dijadwalkan karena koneksi aktif terputus sementara.

Sebagai jalur langsung sebelum restart, panduan ringkas telah disimpan sebagai catatan global berjudul **Panduan baca tulis Memona untuk AI agent**, ID `f2bb06a76542d244e3441a3b86e52b2e`. Hanya key yang memiliki read scope global yang dapat membaca catatan itu. Dokumentasi umum dari tool/resource setelah restart tidak membutuhkan global scope.

## Verifikasi pengembangan

Test memeriksa tool get_memory_guide, resource discovery/read, dan pesan khusus penolakan fact + active. make check, make build, dan smoke test lulus. Koneksi server lama tetap dapat melayani delapan tool sampai direstart; jangan mengklaim tool baru sudah terpasang pada proses lama.

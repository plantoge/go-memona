# Akses jaringan dan HTTPS

Memona mengizinkan listener selain localhost, termasuk `0.0.0.0:8765`, IP Tailscale spesifik, atau `[::]:8765`. Konfigurasi default tetap loopback; `config.json` aktif tidak diganti otomatis.

## Contoh

Lihat `config.network.example.json`:

```json
{
  "listen": "0.0.0.0:8765",
  "data_dir": "./data",
  "public_url": "https://memona.example.com",
  "secure_cookies": true
}
```

Ganti hostname contoh dengan alamat HTTPS sebenarnya sebelum menjalankan. Jangan menggunakan `0.0.0.0` sebagai URL browser; itu alamat binding saja. Gunakan hostname HTTPS pada `public_url`, termasuk port bila bukan 443.

## Jalur koneksi

```text
Browser/harness -> HTTPS reverse proxy -> HTTP Memona:8765
```

Memona saat ini tidak melayani TLS langsung dan tidak membaca sertifikat. `public_url` HTTPS tidak mengubah port backend menjadi HTTPS. Reverse proxy seperti Cloudflare Tunnel atau Tailscale Serve wajib melakukan terminasi TLS dan meneruskan Host asli yang cocok. Host/Origin lain tetap ditolak, bukan diizinkan seluruhnya.

Untuk tunnel yang berada pada mesin sama, loopback biasanya lebih aman dan tetap cukup. Binding `0.0.0.0` berguna jika proxy datang lewat jaringan; binding IP Tailscale spesifik membatasi interface dibanding semua IPv4. IPv6 semua interface memerlukan alamat `[::]` dan perlu pemeriksaan firewall IPv6 terpisah.

## Testing langsung LAN/Tailscale

Untuk akses HTTP privat tanpa reverse proxy, aktifkan opsi eksplisit:

```json
{
  "listen": "0.0.0.0:8765",
  "data_dir": "./data",
  "public_url": "http://100.76.35.38:8765",
  "secure_cookies": false,
  "allow_private_http": true
}
```

Ganti IP dengan alamat server sendiri. Mode ini menerima Host berupa IP privat, loopback, atau rentang Tailscale 100.64.0.0/10 pada port listener. Origin harus persis alamat yang dibuka browser; hostname/IP publik dan port lain ditolak. Login/API key/CSRF tetap aktif. Ini bukan firewall: HTTP melalui LAN tidak terenkripsi, dan pemalsuan Host tidak mencegah akses backend. Jangan port-forward HTTP ini ke internet. Untuk produksi HTTPS nonaktifkan opsi ini (`false`), gunakan URL HTTPS dan secure cookie. Default opsi adalah false.

## Pengamanan yang tetap wajib

- HTTP publik tidak diizinkan; HTTP privat memerlukan `allow_private_http: true`. Secure cookies wajib untuk HTTPS.
- API key dan login admin tidak dimatikan. CORS wildcard tidak ditambahkan.
- Listener wildcard dapat dijangkau LAN/Tailscale sesuai routing dan firewall. Host allowlist bukan firewall: pihak lain dapat memalsukan Host. Batasi port backend ke proxy tepercaya, dan jangan port-forward backend HTTP langsung ke internet.
- Tailscale mengenkripsi koneksi tailnet tetapi tidak otomatis memberikan endpoint HTTPS browser; gunakan Serve atau proxy TLS bila memilih konfigurasi ini.
- Tidak ada perubahan firewall, DNS, router, VPN, proxy, atau service permanen yang dilakukan oleh perubahan kode ini.
- Mengaktifkan konfigurasi listener pada service aktif memerlukan restart terencana; simpan konfigurasi sebelumnya untuk rollback. Kembalikan listen ke loopback jika akses jaringan tidak lagi diperlukan.

## Verifikasi

`internal/server/config_test.go` menguji wildcard/IP Tailscale/IPv6 dengan HTTPS, penolakan HTTP nonlokal dan cookie tidak aman, serta Host/Origin yang salah. Itu pengujian konfigurasi dan handler, bukan bukti akses dari kantor. Pengujian jaringan aktual memerlukan hostname/proxy yang nyata dan disetujui.

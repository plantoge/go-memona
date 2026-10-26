"""One-time local bootstrap; no default password or public registration."""

import getpass
import ipaddress
import json
import os
import sys
from urllib.parse import urlsplit

import local


def network_config():
    print("Akses: [1] Lokal (default)  [2] LAN/Tailscale privat  [3] HTTPS lewat reverse proxy")
    choice = input("Pilih 1/2/3: ").strip() or "1"
    config = {"listen": "127.0.0.1:8765", "data_dir": "./data",
              "public_url": "http://127.0.0.1:8765", "secure_cookies": False}
    if choice == "2":
        raw = input("IP privat server (contoh 192.168.1.10 atau IP Tailscale): ").strip()
        try:
            address = ipaddress.IPv4Address(raw)
        except ipaddress.AddressValueError as exc:
            raise ValueError("Masukkan alamat IPv4 privat server, bukan domain/URL.") from exc
        private_ranges = (ipaddress.ip_network("10.0.0.0/8"), ipaddress.ip_network("172.16.0.0/12"),
                          ipaddress.ip_network("192.168.0.0/16"), ipaddress.ip_network("100.64.0.0/10"))
        if not any(address in network for network in private_ranges):
            raise ValueError("IP harus LAN privat atau Tailscale; IP publik/loopback tidak diizinkan pada mode ini.")
        print("HTTP privat hanya untuk jaringan tepercaya; jangan port-forward ke internet.")
        config.update(listen=f"{address}:8765", public_url=f"http://{address}:8765", allow_private_http=True)
    elif choice == "3":
        raw = input("Alamat HTTPS (contoh https://memona.example.com): ").strip()
        try:
            parsed = urlsplit(raw)
        except ValueError as exc:
            raise ValueError("Alamat HTTPS tidak valid.") from exc
        if (parsed.scheme != "https" or not parsed.hostname or "@" in parsed.netloc
                or parsed.path not in ("", "/") or parsed.query or parsed.fragment or parsed.hostname in ("0.0.0.0", "::")
                or any(c.isspace() for c in raw) or "\\" in raw):
            raise ValueError("Alamat harus origin HTTPS tanpa path, query, fragment, atau kredensial.")
        try:
            _ = parsed.port
        except ValueError as exc:
            raise ValueError("Port pada alamat HTTPS tidak valid.") from exc
        config.update(public_url=raw.rstrip("/"), secure_cookies=True)
        print("Memona tetap di 127.0.0.1:8765. Siapkan proxy TLS dan sertifikat secara terpisah sebelum mengakses dari luar.")
    elif choice != "1":
        raise ValueError("Pilihan akses tidak dikenal; pilih 1, 2, atau 3.")
    return config


def setup(config="config.json"):
    binary = local.ROOT / "bin" / "memona"
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise ValueError("Binary belum ada. Jalankan `make build` dahulu.")
    if not sys.stdin.isatty():
        raise ValueError("Setup awal memerlukan terminal interaktif; tidak menerima password dari argumen.")
    config_file = local.config_path(config)
    exists = config_file.exists()
    if exists:
        path, data, _, _ = local.settings(config)
        print("Config yang sudah ada dipakai tanpa perubahan.")
    else:
        if config_file != local.ROOT / "config.json":
            raise ValueError("Config khusus belum ada; buat dan tinjau lebih dulu.")
        data = local.ROOT / "data"
    # Existing data may already contain a valid admin or an interrupted startup.
    # Never try to reseed or change credentials from this command.
    if data.exists():
        raise ValueError("Data sudah ada. Untuk start rutin gunakan `make start`; setup tidak mereset akun.")
    config_data = None if exists else network_config()
    password = getpass.getpass("Buat password admin (minimal 10 karakter): ")
    repeat = getpass.getpass("Ulangi password: ")
    if password != repeat or not 10 <= len(password.encode("utf-8")) <= 1024:
        raise ValueError("Password tidak cocok atau panjangnya harus 10–1024 byte.")
    env = os.environ.copy()
    env["MEMONA_ADMIN_PASSWORD"] = password
    del password, repeat
    if config_data is not None:
        # Only write a new config after inputs have been validated; never overwrite.
        fd = os.open(config_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as output:
            json.dump(config_data, output, indent=2)
            output.write("\n")
    path = config_file
    print("Admin akan dibuat pada startup pertama. Aplikasi berjalan di foreground; Ctrl+C untuk berhenti.", flush=True)
    os.chdir(local.ROOT)
    os.execve(binary, [str(binary), "serve", "-config", str(path)], env)


if __name__ == "__main__":
    try:
        setup(sys.argv[1] if len(sys.argv) == 2 else "config.json")
    except (ValueError, OSError, EOFError, KeyboardInterrupt) as exc:
        print("Setup Memona: " + str(exc), file=sys.stderr)
        sys.exit(1)

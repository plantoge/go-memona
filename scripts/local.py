#!/usr/bin/env python3
"""Local foreground launcher/status. Never builds, bootstraps, or manages services."""

import argparse
import fcntl
import json
import os
from pathlib import Path
import socket
import sys
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]


def config_path(value):
    path = Path(value)
    return path if path.is_absolute() else ROOT / path


def settings(value):
    path = config_path(value)
    try:
        with path.open(encoding="utf-8") as stream:
            config = json.load(stream)
    except (OSError, ValueError) as exc:
        raise ValueError("Konfigurasi tidak dapat dibaca; periksa path dan JSON tanpa menampilkan isinya.") from exc
    if not isinstance(config, dict):
        raise ValueError("Konfigurasi harus berupa objek JSON.")
    data = config.get("data_dir", "./data")
    if not isinstance(data, str) or not data:
        raise ValueError("data_dir tidak valid.")
    data_path = Path(data)
    if not data_path.is_absolute():
        data_path = ROOT / data_path
    listen = config.get("listen", "127.0.0.1:8765")
    public = config.get("public_url", "http://127.0.0.1:8765")
    if not isinstance(listen, str) or not isinstance(public, str):
        raise ValueError("Alamat layanan tidak valid.")
    return path, data_path, listen, public


def writer_busy(data):
    # Reading and locking the existing file never creates a lock or modifies data.
    lock = data / ".writer.lock"
    try:
        fd = os.open(lock, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    except FileNotFoundError:
        return None
    except OSError as exc:
        raise ValueError("Lock writer tidak dapat diperiksa.") from exc
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return True
        return False
    finally:
        os.close(fd)


def local_health(listen, public):
    """Probe only the local listener; never contact the public URL/network proxy."""
    try:
        host, port = listen.rsplit(":", 1)
        port = int(port)
        if not 1 <= port <= 65535:
            return False
        host = host.strip("[]")
        target = "127.0.0.1" if host == "0.0.0.0" else "::1" if host == "::" else host
        if not (host in ("0.0.0.0", "::", "127.0.0.1", "::1") or host.startswith("127.")):
            return None  # Non-loopback bind: don't probe over network.
        origin = urlsplit(public)
        header_host = origin.netloc
        if not header_host or any(c in header_host for c in "\r\n"):
            return False
        with socket.create_connection((target, port), timeout=1) as sock:
            sock.settimeout(1)
            sock.sendall(("GET /healthz HTTP/1.1\r\nHost: " + header_host + "\r\nConnection: close\r\n\r\n").encode("ascii"))
            response = sock.recv(256)
        return response.startswith(b"HTTP/1.1 200 ") or response.startswith(b"HTTP/1.0 200 ")
    except (OSError, ValueError, UnicodeError):
        return False


def main(argv=None):
    parser = argparse.ArgumentParser(description="Memona lokal: start foreground atau status read-only")
    parser.add_argument("action", choices=("start", "status"))
    parser.add_argument("--config", default="config.json", help="Path konfigurasi relatif ke root proyek")
    args = parser.parse_args(argv)
    try:
        path, data, listen, public = settings(args.config)
        if not data.is_dir():
            raise ValueError("Direktori data belum ada; bootstrap pertama tetap mengikuti README.")
        busy = writer_busy(data)
        if args.action == "status":
            health = local_health(listen, public)
            if busy is True and health is True:
                print("Indikasi Memona aktif: lock writer terpakai dan /healthz lokal merespons. Identitas proses tidak diverifikasi.")
                return 0
            if busy is True:
                print("Lock writer terpakai; kesehatan HTTP tidak terkonfirmasi. Periksa proses sebelum bertindak.")
                return 1
            if health is True:
                print("HTTP lokal merespons, tetapi lock data ini tidak terpakai. Periksa konfigurasi/proses; jangan memulai writer kedua.")
                return 1
            print("Memona tidak terkonfirmasi aktif pada config/data ini; tidak ada file yang diubah.")
            return 1
        binary = ROOT / "bin" / "memona"
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise ValueError("Binary belum tersedia. Bangun sekali dengan `make build` sebelum `make start`.")
        if busy is None:
            raise ValueError("Lock writer tidak ada. Gunakan prosedur bootstrap pertama dalam README; start rutin tidak membuat data baru.")
        if busy is True:
            raise ValueError("Writer sedang aktif. Tidak memulai instance kedua atau menghapus lock.")
        if local_health(listen, public) is True:
            raise ValueError("Port lokal sudah merespons. Periksa proses sebelum memulai instance lain.")
        print("Memulai Memona di foreground; hentikan normal dengan Ctrl+C.", flush=True)
        os.chdir(ROOT)
        os.execv(binary, [str(binary), "serve", "-config", str(path)])
    except (ValueError, OSError) as exc:
        print("Memona: " + str(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())

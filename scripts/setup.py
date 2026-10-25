"""One-time local bootstrap; no default password or public registration."""

import getpass
import os
from pathlib import Path
import sys

import local


def setup(config="config.json"):
    binary = local.ROOT / "bin" / "memona"
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise ValueError("Binary belum ada. Jalankan `make build` dahulu.")
    if not sys.stdin.isatty():
        raise ValueError("Setup awal memerlukan terminal interaktif; tidak menerima password dari argumen.")
    config_file = local.config_path(config)
    if not config_file.exists():
        if config_file != local.ROOT / "config.json":
            raise ValueError("Config khusus belum ada; buat dan tinjau lebih dulu.")
        example = (local.ROOT / "config.example.json").read_bytes()
        # Create only a NEW local-only config. Never overwrite an operator's config.
        fd = os.open(config_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb") as output:
            output.write(example)
    path, data, _, _ = local.settings(config)
    # Existing data may already contain a valid admin or an interrupted startup.
    # Never try to reseed or change credentials from this command.
    if data.exists():
        raise ValueError("Data sudah ada. Untuk start rutin gunakan `make start`; setup tidak mereset akun.")
    password = getpass.getpass("Buat password admin (minimal 16 karakter): ")
    repeat = getpass.getpass("Ulangi password: ")
    if password != repeat or not 16 <= len(password.encode("utf-8")) <= 1024:
        raise ValueError("Password tidak cocok atau panjangnya harus 16–1024 byte.")
    env = os.environ.copy()
    env["MEMONA_ADMIN_PASSWORD"] = password
    del password, repeat
    print("Admin akan dibuat pada startup pertama. Aplikasi berjalan di foreground; Ctrl+C untuk berhenti.", flush=True)
    os.chdir(local.ROOT)
    os.execve(binary, [str(binary), "serve", "-config", str(path)], env)


if __name__ == "__main__":
    try:
        setup(sys.argv[1] if len(sys.argv) == 2 else "config.json")
    except (ValueError, OSError) as exc:
        print("Setup Memona: " + str(exc), file=sys.stderr)
        sys.exit(1)

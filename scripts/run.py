"""Build and run Memona from source, preserving existing config and admin."""

import subprocess
import sys

import local
import setup


def run(config="config.json"):
    path = local.config_path(config)
    if path.exists():
        _, data, _, _ = local.settings(config)
        if data.exists() and not data.is_dir():
            raise ValueError("Target data bukan direktori; periksa config sebelum melanjutkan.")
        if data.is_dir() and local.writer_busy(data) is True:
            raise ValueError("Writer sedang aktif. Tidak membangun ulang atau menjalankan instance kedua.")
        fresh = not data.exists()
    else:
        if path != local.ROOT / "config.json":
            raise ValueError("Config khusus belum ada; buat dan tinjau sebelum menjalankan.")
        if (local.ROOT / "data").exists():
            raise ValueError("Config belum ada tetapi data sudah ada; periksa lokasi config lama. Tidak membuat akun baru.")
        fresh = True

    if fresh and not sys.stdin.isatty():
        raise ValueError("Instalasi baru membutuhkan terminal interaktif untuk password admin.")

    print("Membangun Svelte dan Go…", flush=True)
    subprocess.run(["make", "build"], cwd=local.ROOT, check=True)
    if fresh:
        setup.setup(config)
    else:
        return local.main(["start", "--config", config])
    return 0


if __name__ == "__main__":
    try:
        sys.exit(run(sys.argv[1] if len(sys.argv) == 2 else "config.json"))
    except (ValueError, OSError, subprocess.CalledProcessError) as exc:
        print("Memona: " + str(exc), file=sys.stderr)
        sys.exit(1)

#!/usr/bin/env python3
"""Exercise the real binary and offline tooling using new synthetic data only."""
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

import recovery

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "bin" / "memona"
work = Path(tempfile.mkdtemp(prefix="recovery-smoke-", dir=ROOT / ".cache" / "tmp"))
password = secrets.token_urlsafe(32)
env = os.environ.copy()
env["MEMONA_ADMIN_PASSWORD"] = password
process = None
log = open(work / "server.log", "ab")
opener = None
origin = ""


def config_for(data, name):
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    cfg = {"listen": f"127.0.0.1:{port}", "public_url": f"http://127.0.0.1:{port}",
           "data_dir": str(data), "secure_cookies": False}
    path = work / name
    path.write_text(json.dumps(cfg))
    return path, port


def start(config):
    global process, opener, origin
    origin = json.loads(config.read_text())["public_url"]
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    process = subprocess.Popen([str(BINARY), "serve", "-config", str(config)], cwd=ROOT,
                               env=env, stdout=log, stderr=log)
    for _ in range(100):
        if process.poll() is not None:
            raise RuntimeError("Test binary stopped; inspect retained server.log.")
        try:
            with urllib.request.urlopen(origin + "/healthz", timeout=1) as response:
                if response.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            time.sleep(0.05)
    raise RuntimeError("Test binary startup timed out.")


def stop():
    if process is not None and process.poll() is None:
        process.terminate()
        process.wait(timeout=15)


def request(path, body=None, headers=None):
    headers = dict(headers or {})
    headers.setdefault("Accept", "application/json")
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    with opener.open(urllib.request.Request(origin + path, data=data, headers=headers), timeout=10) as response:
        return json.loads(response.read())


def expect_status(code, action):
    try:
        action()
    except urllib.error.HTTPError as error:
        assert error.code == code, "Unexpected HTTP error status"
    else:
        raise AssertionError("Expected rejection did not happen")


def tool(token, name, args, error=False):
    response = request("/mcp", {"jsonrpc": "2.0", "id": 1, "method": "tools/call",
                               "params": {"name": name, "arguments": args}},
                       {"Authorization": "Bearer " + token, "Accept": "application/json, text/event-stream"})
    result = response["result"]
    assert bool(result.get("isError")) == error, "Unexpected tool outcome; secrets omitted"
    return None if error else json.loads(result["content"][0]["text"])


def fields(kind, title, **extra):
    return {"project": "recovery-demo", "kind": kind, "title": title,
            "content": "Synthetic recovery acceptance fixture.", "status": "draft" if kind == "skill" else "unverified",
            "source": "isolated recovery smoke", **extra}


def tree_hash(path):
    with recovery.directory(path) as fd:
        return recovery.scan(fd)


try:
    config, _ = config_for(work / "source-data", "source.json")
    start(config)
    csrf = {"X-CSRF-Token": request("/api/login", {"password": password})["csrf"]}
    keys = {}
    for name in ("valid", "revoked-before", "revoked-after"):
        keys[name] = request("/api/keys", {"name": "recovery-" + name, "permissions": ["read", "write", "archive"],
                                           "projects": ["recovery-demo"]}, csrf)
    token = keys["valid"]["token"]
    original_args = fields("decision", "Original decision", operation_id="recovery-retry-001")
    original_note = tool(token, "save_memory", original_args)
    assert tool(token, "save_memory", original_args) == original_note
    note = original_note
    note = tool(token, "revise_memory", {**fields("decision", "Revised decision"), "id": note["id"], "expected_revision": 1})
    archived = tool(token, "save_memory", fields("fact", "Archived fixture"))
    tool(token, "archive_memory", {"id": archived["id"], "expected_revision": 1})
    skill = tool(token, "save_memory", fields("skill", "Published fixture"))
    request("/api/notes/" + skill["id"] + "/publish", {"expected_revision": 1}, csrf)
    overview = tool(token, "save_memory", fields("knowledge", "Overview fixture", is_project_summary=True, related_to=[skill["id"]], supersedes=[note["id"]]))
    request("/api/keys/" + keys["revoked-before"]["key"]["id"] + "/revoke", {}, csrf)
    expected_detail = {n["id"]: request("/api/notes/" + n["id"]) for n in (note, archived, skill, overview)}
    expected_activity = request("/api/activity")
    # A live server must prevent backup even if --stopped was incorrectly supplied.
    try:
        recovery.backup(config, BINARY, work / "must-not-exist", stopped=True)
        raise AssertionError("Backup ignored live writer")
    except recovery.RecoveryError:
        assert not (work / "must-not-exist").exists()
    stop()
    before = tree_hash(work / "source-data")
    bundle = work / "backup"
    digest = recovery.backup(config, BINARY, bundle, stopped=True)
    recovery.verify(bundle, digest)
    assert tree_hash(work / "source-data") == before, "Backup altered source"
    # Revoke after the snapshot: a restored copy must reflect backup time, not now.
    env.pop("MEMONA_ADMIN_PASSWORD", None)
    start(config)
    csrf = {"X-CSRF-Token": request("/api/login", {"password": password})["csrf"]}
    request("/api/keys/" + keys["revoked-after"]["key"]["id"] + "/revoke", {}, csrf)
    stop()
    after_source = tree_hash(work / "source-data")
    _, restore_port = config_for(work / "unused", "port-reservation.json")
    restored_config = recovery.restore(bundle, work / "restored", restore_port, digest)
    assert tree_hash(work / "restored" / "data") == before, "Restored bytes differ"
    assert tree_hash(work / "source-data") == after_source, "Restore touched source"
    assert hashlib.sha256((bundle / "memona").read_bytes()).digest() == hashlib.sha256(BINARY.read_bytes()).digest()
    # Use the reviewed project binary with the verified matching hash. Never
    # chmod/execute an arbitrary backup payload from an untrusted source.
    start(restored_config)
    csrf = {"X-CSRF-Token": request("/api/login", {"password": password})["csrf"]}
    for id, expected in expected_detail.items():
        assert request("/api/notes/" + id) == expected, "Restored note/history/relations mismatch"
    assert tool(token, "save_memory", original_args) == original_note, "Restored idempotency receipt lost"
    current_activity = request("/api/activity")
    for event in expected_activity:
        assert event in current_activity, "Audit event missing after restore"
    assert tool(token, "get_skill", {"id": skill["id"]})["status"] == "active"
    tool(token, "read_memory", {"id": note["id"]})
    tool(token, "get_context", {"project": "outside-scope"}, error=True)
    expect_status(401, lambda: tool(keys["revoked-before"]["token"], "read_memory", {"id": note["id"]}))
    tool(keys["revoked-after"]["token"], "read_memory", {"id": note["id"]})
    restored = request("/api/notes/" + archived["id"] + "/restore", {"expected_revision": 2}, csrf)
    assert restored["revision"] == 3 and not restored["archived"]
    # Restored instance is independent: original bytes remain exactly unchanged.
    assert tree_hash(work / "source-data") == after_source, "Acceptance changed original data"
    print("PASS: live-writer refusal, offline backup/checksums/private files, isolated restore, unchanged source, admin login, key scope/revocation snapshot, notes/history/relations/archive/skill/audit, idempotency replay after restore.")
    print("Retained synthetic fixture directory:", work)
finally:
    stop()
    log.close()

#!/usr/bin/env python3
"""Isolated binary smoke test; synthetic fixtures are retained, never auto-deleted."""
import http.cookiejar
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
work = Path(tempfile.mkdtemp(prefix="smoke-", dir=ROOT / ".cache" / "tmp"))
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
origin = f"http://127.0.0.1:{port}"
config = work / "config.json"
config.write_text(json.dumps({"listen": f"127.0.0.1:{port}", "data_dir": str(work / "data"), "public_url": origin, "secure_cookies": False}))
password = secrets.token_urlsafe(32)
env = os.environ.copy()
env["MEMONA_ADMIN_PASSWORD"] = password
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
process = None
log = open(work / "server.log", "ab")


def start():
    proc = subprocess.Popen([str(ROOT / "bin" / "memona"), "serve", "-config", str(config)], cwd=ROOT, env=env, stdout=log, stderr=log)
    for _ in range(100):
        if proc.poll() is not None:
            raise RuntimeError("binary stopped; inspect retained server.log")
        try:
            with urllib.request.urlopen(origin + "/healthz", timeout=1) as response:
                if response.status == 200:
                    return proc
        except (OSError, urllib.error.URLError):
            time.sleep(0.05)
    proc.terminate()
    proc.wait(timeout=10)
    raise RuntimeError("startup timed out")


def request(path, body=None, headers=None):
    h = dict(headers or {})
    h.setdefault("Accept", "application/json")
    data = None
    if body is not None:
        h["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    with opener.open(urllib.request.Request(origin + path, data=data, headers=h), timeout=10) as response:
        raw = response.read()
        return json.loads(raw) if raw else None


def rpc(token, method, params, number=1):
    return request("/mcp", {"jsonrpc": "2.0", "id": number, "method": method, "params": params}, {"Authorization": "Bearer " + token, "Accept": "application/json, text/event-stream"})


try:
    process = start()
    with urllib.request.urlopen(origin + "/") as response:
        page = response.read().decode()
        assert "script-src 'self'" in response.headers["Content-Security-Policy"]
    assets = re.findall(r'(?:src|href)="(/assets/[^\"]+)"', page)
    assert len(assets) >= 2, "built frontend assets missing"
    for asset in assets:
        with urllib.request.urlopen(origin + asset) as response:
            assert response.status == 200 and len(response.read()) > 0
        with urllib.request.urlopen(urllib.request.Request(origin + asset, method="HEAD")) as response:
            assert response.read() == b"", "HEAD returned body"
    # Exercise the lazy graph asset URL as an embedded static file, not as a
    # browser/WebGL acceptance claim. The current index references this import.
    entry_js = next(asset for asset in assets if asset.endswith('.js'))
    with urllib.request.urlopen(origin + entry_js) as response:
        entry_source = response.read().decode()
    graph_assets = set(re.findall(r'three-graph-renderer-[A-Za-z0-9_-]+\.js', entry_source))
    assert graph_assets, 'lazy graph chunk reference missing'
    for asset in graph_assets:
        with urllib.request.urlopen(origin + '/assets/' + asset) as response:
            assert response.status == 200 and len(response.read()) > 0
    for private_path in ["/config.json", "/frontend/src/App.svelte", "/data/", "/app.js"]:
        try:
            urllib.request.urlopen(origin + private_path)
            raise AssertionError("private or legacy path exposed")
        except urllib.error.HTTPError as error:
            assert error.code == 404
    try:
        request("/api/storage-health")
        raise AssertionError("unauthenticated diagnostics exposed")
    except urllib.error.HTTPError as error:
        assert error.code == 401
    session = request("/api/login", {"password": password})
    csrf = {"X-CSRF-Token": session["csrf"]}
    info = request("/api/server-info")
    assert info["mcp_endpoint"] == origin + "/mcp", "wrong configured MCP endpoint"
    assert "token" not in info and "password" not in info
    created = request("/api/keys", {"name": "binary-smoke", "permissions": ["read", "write"], "projects": ["smoke-test"]}, csrf)
    token = created["token"]
    assert token not in json.dumps(request("/api/keys")), "key list leaked secret"
    initialized = rpc(token, "initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "smoke", "version": "1"}})
    assert "result" in initialized, "MCP initialize failed"
    tools = rpc(token, "tools/list", {})["result"]["tools"]
    assert len(tools) == 11, "unexpected tools"
    for tool in tools:
        if tool["name"] in ("save_memory", "save_handoff", "revise_memory", "archive_memory"):
            assert "operation_id" in tool["inputSchema"]["properties"]
            assert "operation_id" not in tool["inputSchema"].get("required", [])

    def mutation(name, arguments, error=False):
        result = rpc(token, "tools/call", {"name": name, "arguments": arguments})["result"]
        assert bool(result.get("isError")) == error, "unexpected mutation outcome"
        if not error:
            output = json.loads(result["content"][0]["text"])
            assert "_operation" not in json.dumps(output), "internal receipt leaked"
            return output

    retry_args = {"project": "smoke-test", "kind": "fact", "title": "Retry fixture", "content": "Synthetic retry.", "status": "unverified", "source": "", "operation_id": "smoke-retry-001", "is_testing": True}
    retry_note = mutation("save_memory", retry_args)
    assert mutation("save_memory", retry_args) == retry_note
    candidates = mutation("find_duplicate_candidates", {"project":"smoke-test", "title":retry_args["title"], "content":retry_args["content"]})
    assert candidates["candidates"][0]["id"] == retry_note["id"]
    mutation("save_memory", {**retry_args, "content": "Different payload"}, error=True)
    handoff_args = {**retry_args, "kind": "handoff"}
    handoff_note = mutation("save_handoff", handoff_args)
    assert handoff_note["id"] != retry_note["id"]
    revise_args = {**retry_args, "id": retry_note["id"], "expected_revision": 1, "content": "Revised retry."}
    retry_revision = mutation("revise_memory", revise_args)
    assert mutation("revise_memory", revise_args) == retry_revision
    result = rpc(token, "tools/call", {"name": "save_memory", "arguments": {"project": "smoke-test", "kind": "decision", "title": "Binary smoke fixture", "content": "Synthetic persistence check.", "status": "verified", "source": "isolated smoke fixture", "verified_at":"2026-10-23T09:00:00Z", "needs_review":True}})["result"]
    assert not result.get("isError"), "save failed"
    note = json.loads(result["content"][0]["text"])
    detail = request("/api/notes/" + note["id"])
    assert detail["note"]["content"] == "Synthetic persistence check."
    assert detail["note"]["verified_at"] == "2026-10-23T09:00:00Z" and detail["note"]["needs_review"] is True
    result = rpc(token, "tools/call", {"name": "save_memory", "arguments": {"project": "smoke-test", "kind": "knowledge", "title": "Project overview fixture", "content": "Synthetic project overview.", "status": "unverified", "source": "", "is_project_summary": True, "supersedes": [note["id"]]}})["result"]
    assert not result.get("isError"), "summary/relation save failed"
    overview = json.loads(result["content"][0]["text"])
    links = rpc(token, "tools/call", {"name": "get_memory_relations", "arguments": {"id": note["id"]}})["result"]
    assert not links.get("isError"), "relations read failed"
    assert json.loads(links["content"][0]["text"])["superseded_by"][0]["id"] == overview["id"]
    context = rpc(token, "tools/call", {"name": "get_context", "arguments": {"project": "smoke-test", "limit": 1}})["result"]
    assert not context.get("isError"), "project context failed"
    assert json.loads(context["content"][0]["text"])["notes"][0]["id"] == overview["id"]
    assert json.loads(context["content"][0]["text"])["selection_reasons"][0]["id"] == overview["id"]
    mixed = mutation("get_context", {"project":"smoke-test"})
    assert retry_note["id"] not in {n["id"] for n in mixed["notes"]}
    included = mutation("get_context", {"project":"smoke-test", "include_testing":True})
    assert retry_note["id"] in {n["id"] for n in included["notes"]}
    overview = request("/api/notes/" + overview["id"] + "/connections", {"expected_revision": 1, "is_project_summary": True, "related_to": [note["id"]], "supersedes": []}, csrf)
    assert overview["revision"] == 2
    assert request("/api/notes/" + note["id"])["note"]["connections"]["referenced_by"][0]["id"] == overview["id"]
    process.terminate()
    process.wait(timeout=10)
    env.pop("MEMONA_ADMIN_PASSWORD", None)
    process = start()
    session = request("/api/login", {"password": password})
    csrf = {"X-CSRF-Token": session["csrf"]}
    assert request("/api/notes/" + note["id"])["note"]["revision"] == 1
    assert request("/api/notes/" + note["id"])["note"]["needs_review"] is True
    persisted = request("/api/notes/" + overview["id"])["note"]
    assert persisted["revision"] == 2 and persisted["is_project_summary"]
    assert persisted["related_to"] == [note["id"]] and not persisted.get("supersedes")
    assert mutation("save_memory", retry_args) == retry_note
    assert mutation("save_handoff", handoff_args) == handoff_note
    assert mutation("revise_memory", revise_args) == retry_revision
    assert request("/api/notes/" + retry_note["id"])["note"]["is_testing"] is True
    assert request("/api/notes/" + retry_note["id"])["note"]["revision"] == 2
    request("/api/keys/" + created["key"]["id"] + "/revoke", {}, csrf)
    try:
        rpc(token, "tools/list", {})
        raise AssertionError("revoked key accepted")
    except urllib.error.HTTPError as error:
        assert error.code == 401

    security = request('/api/request-security')
    assert security['counters']['auth_failures'] >= 1
    assert security['counters']['audit_sampled'] >= 1
    assert 'append-only' in security['audit_retention']
    assert token not in json.dumps(security)

    def snapshot():
        return {str(p.relative_to(work / "data")): (p.stat().st_mode, hashlib.sha256(p.read_bytes()).hexdigest())
                for p in (work / "data").rglob("*") if p.is_file()}

    before = snapshot()
    health = request("/api/storage-health")
    assert health["integrity"]["complete"] and not health["integrity"]["issues"]
    assert health["disk"]["available"] and health["write_failures_observed"]
    assert health["integrity"]["notes"] == 4
    after_health = snapshot()
    changed_health = sorted(name for name in set(before) | set(after_health) if before.get(name) != after_health.get(name))
    # Session authentication reads SQLite: its SHM reader coordination bytes can
    # change without changing durable data. Keep DB/WAL/Markdown/staging checks;
    # offline inspect below still checks the entire tree byte-for-byte.
    assert not [name for name in changed_health if name != 'memona.db-shm'], "online health changed durable stored bytes"
    inspect_args = [str(ROOT / "bin" / "memona"), "inspect", "-data-dir", str(work / "data")]
    busy = subprocess.run(inspect_args, capture_output=True, timeout=10)
    assert busy.returncode == 1 and not busy.stdout and b"writer active" in busy.stderr
    # Synthetic failure injection only; retain evidence, never repair/delete.
    stage = work / "data" / ".staging" / "PRIVATE_STAGE_NAME"
    stage.write_text("PRIVATE_STAGE_BODY")
    stage.chmod(0o600)
    corrupt = work / "data" / "notes" / note["id"] / "000002.md"
    corrupt.write_text("PRIVATE_CORRUPT_BODY")
    corrupt.chmod(0o600)
    before = snapshot()
    health = request("/api/storage-health")
    assert health["status"] == "error" and health["integrity"]["issues"]["corrupt_revision"] == 1
    assert health["staging_entries"] == 1 and "PRIVATE" not in json.dumps(health)
    assert request("/healthz") == {"status": "ok"}, "liveness exposed readiness/details"
    assert snapshot() == before, "diagnostic changed corrupt evidence"
    process.terminate()
    process.wait(timeout=10)
    # Shutdown can checkpoint SQLite; compare only after normal shutdown.
    before = snapshot()
    offline = subprocess.run(inspect_args, capture_output=True, timeout=10)
    assert offline.returncode == 2 and not offline.stderr
    health = json.loads(offline.stdout)
    assert health["integrity"]["issues"]["corrupt_revision"] == 1 and not health["write_failures_observed"]
    assert "PRIVATE" not in offline.stdout.decode() and snapshot() == before
    print("PASS: embedded Svelte assets/CSP/HEAD/private paths, binary startup, admin login, UI key API, MCP initialize/tools/save, summary priority/supersedes/backlinks/admin metadata, operation_id discovery/retry/conflict/handoff/revise/restart persistence, revocation, admin health/read-only snapshot, offline inspect writer refusal/corruption/exit2/staging retained, minimal public liveness.")
    print("Retained synthetic fixture directory:", work)
finally:
    if process is not None and process.poll() is None:
        process.terminate()
        process.wait(timeout=10)
    log.close()

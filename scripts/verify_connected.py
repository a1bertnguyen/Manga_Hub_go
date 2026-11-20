"""Repeatable grade-readiness checks against separate backend/catalog processes.

Run from the repository root: python scripts/verify_connected.py
Only temporary databases, random loopback ports and temporary demo accounts are used.
Exit 1 means a required behavior failed, including an existing implementation gap.
"""

import base64
import concurrent.futures
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import secrets
import socket
import struct
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "mangahub-main" / "MangaHub-main"


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def exact(sock, count):
    result = b""
    while len(result) < count:
        chunk = sock.recv(count - len(result))
        if not chunk:
            raise EOFError("Socket closed")
        result += chunk
    return result


def line(sock):
    result = b""
    while not result.endswith(b"\n"):
        result += exact(sock, 1)
    return json.loads(result)


def ws_receive(sock):
    header = exact(sock, 2)
    length = header[1] & 127
    if length == 126:
        length = struct.unpack("!H", exact(sock, 2))[0]
    elif length == 127:
        length = struct.unpack("!Q", exact(sock, 8))[0]
    return json.loads(exact(sock, length))


def ws_send(sock, message):
    data = json.dumps(message).encode()
    mask = secrets.token_bytes(4)
    header = bytes([0x81, 0x80 | len(data)]) if len(data) < 126 else bytes([0x81, 0xFE]) + struct.pack("!H", len(data))
    sock.sendall(header + mask + bytes(byte ^ mask[i % 4] for i, byte in enumerate(data)))


def run():
    results = []


    def record(name, passed, detail):
        results.append({"check": name, "passed": passed, "observed": detail})

    with tempfile.TemporaryDirectory(prefix="mangahub-review-") as temp:
        folder = Path(temp)
        extension = ".exe" if os.name == "nt" else ""
        for command in ("catalog", "backend"):
            subprocess.run(["go", "build", "-o", str(folder / (command + extension)), "./cmd/" + command], cwd=MODULE, check=True)
        http_port, tcp_port, udp_port, grpc_port = [free_port() for _ in range(4)]
        env = dict(os.environ, HTTP_PORT=str(http_port), TCP_PORT=str(tcp_port), UDP_PORT=str(udp_port), GRPC_PORT=str(grpc_port),
                   CATALOG_GRPC_ADDR=f"127.0.0.1:{grpc_port}", CATALOG_DB_PATH=str(folder / "catalog.db"),
                   BACKEND_DB_PATH=str(folder / "backend.db"), JWT_SECRET=secrets.token_hex(32), ADMIN_API_KEY=secrets.token_hex(32))
        base = f"http://127.0.0.1:{http_port}"
        processes, sockets, logs = {}, [], []

        def start(name):
            log = open(folder / f"{name}-{len(logs)}.log", "wb")
            logs.append(log)
            processes[name] = subprocess.Popen([str(folder / (name + extension))], cwd=MODULE, env=env, stdout=log, stderr=subprocess.STDOUT)

        def stop(name):
            process = processes.pop(name, None)
            if process:
                process.terminate()
                process.wait(timeout=10)

        def request(path, token=None, method="GET", data=None, admin=False):
            headers = {"Content-Type": "application/json"}
            if admin:
                headers["X-Admin-Key"] = env["ADMIN_API_KEY"]
            if token:
                headers["Authorization"] = "Bearer " + token
            # Keep HTTP/1.1 alive until the response is consumed. urllib forces
            # Connection: close; on Windows, an early 401/403 can reset that socket
            # while its request body is still being drained by net/http.
            connection = http.client.HTTPConnection("127.0.0.1", http_port, timeout=8)
            try:
                connection.request(method, path, body=None if data is None else json.dumps(data), headers=headers)
                response = connection.getresponse()
                return response.status, json.loads(response.read())
            finally:
                connection.close()

        def wait_ready():
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                try:
                    code, data = request("/api/health")
                    if code == 200 and data["services"]["grpc_catalog"] == "connected":
                        return
                except (OSError, ValueError):
                    pass
                time.sleep(0.1)
            raise RuntimeError("Independent services did not become ready")

        def tcp_auth(payload):
            sock = socket.create_connection(("127.0.0.1", tcp_port), timeout=2)
            sockets.append(sock)
            sock.sendall(json.dumps(payload).encode() + b"\n")
            return sock, line(sock)

        def websocket(username, room, token=None):
            sock = socket.create_connection(("127.0.0.1", http_port), timeout=2)
            sockets.append(sock)
            key = base64.b64encode(secrets.token_bytes(16)).decode()
            path = f"/ws?username={username}&room={room}"
            authorization = f"Authorization: Bearer {token}\r\n" if token else ""
            sock.sendall((f"GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{http_port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: {key}\r\n{authorization}\r\n").encode())
            header = b""
            while not header.endswith(b"\r\n\r\n"):
                header += exact(sock, 1)
            return sock, int(header.split(b" ")[1])

        try:
            start("catalog")
            start("backend")
            wait_ready()
            record("Independent processes", True, "Go catalog and Gin backend started with separate temporary SQLite files")
            suffix = secrets.token_hex(4)
            accounts = []
            for name in ("alice", "bob"):
                code, account = request("/api/auth/register", method="POST", data={"username": name + suffix, "email": f"{name}{suffix}@example.test", "password": "Review-only-" + suffix})
                if code != 200:
                    raise RuntimeError(f"Registration failed: HTTP {code}")
                accounts.append(account)
            alice, bob = accounts
            token = alice["token"]
            code, _ = request("/api/admin/manga/any/release", method="POST", data={"chapter": 1})
            record("Admin requires login", code == 401, f"HTTP {code}")
            code, _ = request("/api/admin/manga/any/release", token, "POST", {"chapter": 1})
            record("Admin requires configured key", code == 403, f"HTTP {code}")
            code, catalog = request("/api/manga", token)
            manga = catalog["manga"][0]
            manga_id, total = manga["id"], manga["total_chapters"]
            record("HTTP -> gRPC catalog", code == 200 and len(catalog["manga"]) >= 30, f"HTTP {code}; {len(catalog['manga'])} seeded titles")
            code, _ = request("/api/library")
            record("Unauthorized REST", code == 401, f"Unauthenticated library request: HTTP {code}")
            request("/api/library", token, "POST", {"manga_id": manga_id, "status": "reading"})
            a1, _ = tcp_auth({"type": "auth", "token": token})
            a2, _ = tcp_auth({"type": "auth", "token": token})
            b, _ = tcp_auth({"type": "auth", "token": bob["token"]})
            time.sleep(0.1)
            code, _ = request(f"/api/library/{manga_id}/progress", token, "PUT", {"current_chapter": 1, "status": "reading"})
            events = [line(a1), line(a2)]
            isolated = False
            b.settimeout(0.3)
            try:
                line(b)
            except socket.timeout:
                isolated = True
            record("TCP push and user isolation", code == 200 and all(e.get("chapter") == 1 for e in events) and isolated,
                   "Two same-user sockets received chapter 1; different-user socket received nothing" if isolated else "Different-user socket received data")
            _, forged = tcp_auth({"type": "auth", "user_id": alice["user_id"]})
            record("TCP requires verified identity", forged.get("type") != "authenticated", f"Tokenless user_id accepted: {forged.get('type') == 'authenticated'}")
            _, verified = tcp_auth({"type": "auth", "token": token, "user_id": bob["user_id"]})
            record("TCP ignores forged user_id with valid JWT", verified.get("user_id") == alice["user_id"], "Identity derived from the JWT claims")
            encode = lambda value: base64.urlsafe_b64encode(json.dumps(value, separators=(",", ":")).encode()).rstrip(b"=")
            signing_input = encode({"alg": "HS256", "typ": "JWT"}) + b"." + encode({"user_id": alice["user_id"], "username": alice["username"], "exp": int(time.time()) - 60})
            expired = (signing_input + b"." + base64.urlsafe_b64encode(hmac.new(env["JWT_SECRET"].encode(), signing_input, hashlib.sha256).digest()).rstrip(b"=")).decode()
            expired_http, _ = request("/api/library", expired)
            _, expired_tcp = tcp_auth({"type": "auth", "token": expired})
            _, expired_ws = websocket("alice", "general", expired)
            record("Expired JWT rejected across protocols", expired_http == 401 and expired_ws == 401 and expired_tcp.get("type") == "error", "Expired JWT rejected by REST, TCP and WebSocket")
            def concurrent_update(chapter):
                return request(f"/api/library/{manga_id}/progress", token, "PUT", {"current_chapter": chapter})
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                updates = list(pool.map(concurrent_update, [index % max(total, 1) for index in range(8)]))
            versions = [body.get("version") for code, body in updates]
            _, snapshot = request("/api/library", token)
            last = max((body for code, body in updates if code == 200), key=lambda body: body["version"], default={})
            item = snapshot["library"][0]
            record("Concurrent progress commits", all(code == 200 for code, body in updates) and len(set(versions)) == 8 and item["version"] == last.get("version") and item["current_chapter"] == last.get("chapter"), "Eight concurrent HTTP updates; unique versions and final stored state checked")
            udp_clients = []
            for _ in range(3):
                sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
                sock.connect(("127.0.0.1", udp_port)); sock.settimeout(2)
                sockets.append(sock); udp_clients.append(sock)
                sock.send(b'{"type":"subscribe"}')
                json.loads(sock.recv(4096))
            udp_clients[2].send(b'{"type":"unsubscribe"}')
            json.loads(udp_clients[2].recv(4096))
            code, _ = request(f"/api/admin/manga/{manga_id}/release", token, "POST", {"chapter": total + 1}, admin=True)
            alerts = [json.loads(sock.recv(4096)) for sock in udp_clients[:2]]
            unsubscribed = False
            udp_clients[2].settimeout(0.3)
            try:
                udp_clients[2].recv(4096)
            except socket.timeout:
                unsubscribed = True
            record("UDP subscriptions and release", code == 200 and all(e.get("type") == "new_chapter" for e in alerts) and unsubscribed,
                   "Two subscribed sockets received a chapter alert; unsubscribed socket received nothing")
            _, status = websocket("alice", "general")
            record("WebSocket rejects missing JWT", status in (401, 403), f"Upgrade without JWT: HTTP {status}")
            wa, _ = websocket("alice", "review-general", token)
            wb, _ = websocket("bob", "review-general", bob["token"])
            wc, _ = websocket("charlie", "review-other", bob["token"])
            marker = "review-" + suffix
            ws_send(wa, {"type": "chat", "text": marker, "room": "review-general"})
            message = ws_receive(wb)
            room_isolated = False
            wc.settimeout(0.3)
            try:
                ws_receive(wc)
            except socket.timeout:
                room_isolated = True
            record("WebSocket room delivery", message.get("text") == marker and message.get("username") == alice["username"] and room_isolated, "Same-room client received chat; other room received nothing")
            if "--with-tui" in sys.argv:
                live_env = dict(env, MANGAHUB_LIVE_API=base, MANGAHUB_LIVE_TCP=f"127.0.0.1:{tcp_port}",
                                MANGAHUB_LIVE_UDP=f"127.0.0.1:{udp_port}", MANGAHUB_LIVE_WS=f"ws://127.0.0.1:{http_port}/ws",
                                MANGAHUB_LIVE_MANGA=manga_id, MANGAHUB_LIVE_TOTAL=str(total))
                for label, account in [("A", alice), ("B", bob)]:
                    for field in ("TOKEN", "ID", "NAME"):
                        key = {"TOKEN": "token", "ID": "user_id", "NAME": "username"}[field]
                        live_env[f"MANGAHUB_LIVE_{label}_{field}"] = account[key]
                completed = subprocess.run(["go", "test", "-race", "./internal/tui", "-run", "^TestLiveConnectedTUI$", "-count=1", "-v", "-timeout=90s"],
                                           cwd=MODULE, env=live_env, capture_output=True, text=True, timeout=120)
                record("Live TUI integration under race detector", completed.returncode == 0, completed.stdout + completed.stderr)
            code, _ = request(f"/api/library/{manga_id}/progress", token, "PUT", {"current_chapter": total + 100})
            record("Chapter overflow rejected", code == 400, f"Above-total update: HTTP {code}")
            stop("catalog")
            started = time.monotonic()
            code, _ = request("/api/manga", token)
            elapsed = time.monotonic() - started
            record("Bounded catalog failure", code == 503 and elapsed < 5, f"HTTP {code} after {elapsed:.3f}s")
            code, _ = request(f"/api/library/{manga_id}/progress", token, "PUT", {"current_chapter": total + 100})
            record("Progress validation during catalog failure", code == 503, f"Catalog stopped; above-total update returned HTTP {code}")
            code, _ = request("/api/library", token, "POST", {"manga_id": "missing-title"})
            record("Library addition during catalog failure", code == 503, f"Catalog stopped; add returned HTTP {code}")
            start("catalog")
            wait_ready()
            request(f"/api/library/{manga_id}/progress", token, "PUT", {"current_chapter": 1, "status": "reading"})
            stop("backend")
            start("backend")
            wait_ready()
            code, library = request("/api/library", token)
            record("Backend restart persistence", code == 200 and library["library"][0]["current_chapter"] == 1,
                   f"Restarted real backend; stored chapter = {library['library'][0]['current_chapter']}")
        except Exception:
            for log in logs:
                log.flush()
            for path in folder.glob("*.log"):
                print(path.name + ":\n" + path.read_text(errors="replace")[-5000:], file=sys.stderr)
            raise
        finally:
            for sock in sockets:
                sock.close()
            for name in list(processes):
                stop(name)
            for log in logs:
                log.close()
    print(json.dumps({"checks": results, "passed": sum(r["passed"] for r in results), "failed": sum(not r["passed"] for r in results)}, indent=2))
    return int(any(not r["passed"] for r in results))


if __name__ == "__main__":
    sys.exit(run())

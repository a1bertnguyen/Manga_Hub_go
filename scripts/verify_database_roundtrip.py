"""Database readback/restart checks used by verify_postman.py.

Only operates on that verifier's disposable services, users and SQLite files.
"""
from contextlib import closing
from http.client import HTTPConnection
import base64
import json
import secrets
import socket
import sqlite3
import subprocess

from verify_connected import MODULE, exact, ws_receive, ws_send


def verify_database_roundtrip(http_port, backend_db, catalog_db, state, env, stop, start, ready):
    checks, sockets = [], []

    def record(name, passed):
        checks.append({"check": name, "passed": bool(passed)})
        if not passed:
            raise AssertionError(name)

    def sql(path, query, parameters=()):
        with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)) as db:
            return db.execute(query, parameters).fetchall()

    def http(path, method="GET", body=None, admin=False):
        connection = HTTPConnection("127.0.0.1", http_port, timeout=8)
        headers = {"Content-Type": "application/json", "Authorization": "Bearer " + state["token"]}
        if admin:
            headers["X-Admin-Key"] = env["ADMIN_API_KEY"]
        try:
            connection.request(method, path, body=json.dumps(body) if body is not None else None, headers=headers)
            response = connection.getresponse()
            return response.status, json.loads(response.read())
        finally:
            connection.close()

    def connect(token, room):
        sock = socket.create_connection(("127.0.0.1", http_port), timeout=5)
        sockets.append(sock)
        key = base64.b64encode(secrets.token_bytes(16)).decode()
        sock.sendall((f"GET /ws?room={room} HTTP/1.1\r\nHost: 127.0.0.1:{http_port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: {key}\r\nAuthorization: Bearer {token}\r\n\r\n").encode())
        header = b""
        while not header.endswith(b"\r\n\r\n"):
            header += exact(sock, 1)
        assert int(header.split(b" ")[1]) == 101
        history = ws_receive(sock)
        assert history["type"] == "history" and history["room"] == room
        return sock, history["messages"]

    try:
        record("Backend SQLite integrity", sql(backend_db, "PRAGMA quick_check") == [("ok",)])
        record("Catalog SQLite integrity", sql(catalog_db, "PRAGMA quick_check") == [("ok",)])
        record("Fresh verifier data has no foreign-key violations", not sql(backend_db, "PRAGMA foreign_key_check"))
        original = sql(backend_db, "SELECT current_chapter,status,version FROM user_progress WHERE user_id=? AND manga_id=?", (state["userId"], state["mangaId"]))
        user_hash = sql(backend_db, "SELECT password_hash FROM users WHERE id=?", (state["userId"],))
        record("Account uses persisted bcrypt hash", bool(user_hash and user_hash[0][0].startswith(("$2a$", "$2b$", "$2y$"))))
        alice, _ = connect(state["token"], "general")
        bob, _ = connect(state["bobToken"], "general")
        other, _ = connect(state["bobToken"], "spoilers")
        marker = "dbchat-" + secrets.token_hex(4)
        text = "Xin chào [red] O'Reilly " + marker
        identifier = "db-audit-" + secrets.token_hex(8)
        ws_send(alice, {"type": "chat", "id": identifier, "username": "forged", "room": "spoilers", "text": text})
        events = [ws_receive(alice), ws_receive(bob)]
        record("Stored chat reaches sender and same-room peer", all(e.get("id") == identifier and e.get("text") == text and e.get("room") == "general" for e in events))
        row = sql(backend_db, "SELECT user_id,username,room,text,created_at FROM chat_messages WHERE id=?", (identifier,))
        record("Echo corresponds to committed SQLite row and verified identity", len(row) == 1 and row[0][:4] == (state["userId"], state["testUsername"], "general", text) and bool(row[0][4]))
        other.settimeout(.2)
        try:
            unexpected = ws_receive(other)
        except socket.timeout:
            unexpected = None
        record("Different room receives no live private-room frame", unexpected is None)
        code, manga = http("/api/manga/" + state["mangaId"])
        chapter = manga["total_chapters"] + 1
        code, _ = http("/api/admin/manga/" + state["mangaId"] + "/release", "POST", {"chapter": chapter}, True)
        record("Release persists through gRPC in catalog SQLite", code == 200 and sql(catalog_db, "SELECT total_chapters FROM manga WHERE id=?", (state["mangaId"],)) == [(chapter,)])
        for sock in sockets:
            sock.close()
        sockets.clear()
        stop("backend"); stop("catalog")
        start("catalog"); start("backend"); ready()
        code, login = http("/api/auth/login", "POST", {"username": state["testUsername"], "password": state["password"]})
        record("Original account logs in after both services restart", code == 200 and login.get("user_id") == state["userId"])
        record("Progress chapter/status/version unchanged after restart", sql(backend_db, "SELECT current_chapter,status,version FROM user_progress WHERE user_id=? AND manga_id=?", (state["userId"], state["mangaId"])) == original)
        code, library = http("/api/library")
        item = next(i for i in library["library"] if i["manga_id"] == state["mangaId"])
        record("HTTP library matches persisted SQLite row", code == 200 and (item["current_chapter"], item["status"], item["version"]) == original[0])
        code, manga = http("/api/manga/" + state["mangaId"])
        record("HTTP catalog returns persisted release after restart", code == 200 and manga["total_chapters"] == chapter)
        _, history = connect(state["token"], "general")
        _, isolated = connect(state["bobToken"], "spoilers")
        record("Stored chat is replayed exactly once after process restart", sum(m.get("id") == identifier and m.get("text") == text for m in history) == 1)
        record("Stored room history remains isolated", not any(m.get("id") == identifier for m in isolated))
        live_env = dict(env, MANGAHUB_HISTORY_MARKER=marker, MANGAHUB_HISTORY_TOKEN=state["token"], MANGAHUB_HISTORY_USER_ID=state["userId"], MANGAHUB_HISTORY_USERNAME=state["testUsername"], MANGAHUB_HISTORY_WS=f"ws://127.0.0.1:{http_port}/ws")
        completed = subprocess.run(["go", "test", "-race", "./internal/tui", "-run", "^TestLiveDiscussionHistoryTUI$", "-count=1", "-v", "-timeout=60s"], cwd=MODULE, env=live_env, capture_output=True, text=True, timeout=90)
        record("Real TUI renders persisted room history after restart", completed.returncode == 0)
        return {"name": "Database roundtrip and restart", "passed": True, "checks": checks, "observed": completed.stdout + completed.stderr}
    finally:
        for sock in sockets:
            sock.close()

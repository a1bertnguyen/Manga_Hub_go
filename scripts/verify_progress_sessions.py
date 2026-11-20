"""F03 source-session regression against real HTTP/TCP services.

Called by verify_postman.py with disposable users and databases. No credentials
or session identifiers are included in the retained result.
"""
from http.client import HTTPConnection
import json
import socket
import time

from verify_connected import line


def verify_progress_sessions(http_port, tcp_port, token, other_token, manga_id):
    sockets = []

    def request(session="", path=None, body=None, identity=None, method="PUT"):
        connection = HTTPConnection("127.0.0.1", http_port, timeout=5)
        headers = {"Content-Type": "application/json", "Authorization": "Bearer " + (identity or token)}
        if session:
            headers["X-Session-ID"] = session
        try:
            connection.request(method, path or f"/api/library/{manga_id}/progress", body=json.dumps(body) if body is not None else None, headers=headers)
            response = connection.getresponse()
            return response.status, json.loads(response.read())
        finally:
            connection.close()

    def connect(identity):
        sock = socket.create_connection(("127.0.0.1", tcp_port), timeout=3)
        sockets.append(sock)
        sock.sendall(json.dumps({"type": "auth", "token": identity}).encode() + b"\n")
        acknowledgement = line(sock)
        assert acknowledgement["type"] == "authenticated" and acknowledgement.get("session_id"), "Missing server-issued TCP session"
        return sock, acknowledgement["session_id"]

    def received(sock, response, chapter):
        sock.settimeout(3)
        event = line(sock)
        assert event["type"] == "progress_updated" and event["manga_id"] == manga_id, "Wrong progress event"
        assert event["chapter"] == chapter and event["version"] == response["version"], "Event does not match committed HTTP response"

    def silent(*receivers):
        for sock in receivers:
            sock.settimeout(.15)
            try:
                event = line(sock)
            except socket.timeout:
                continue
            raise AssertionError(f"Source or unrelated client received a private event: {event.get('type')}")

    def stored():
        code, data = request(path="/api/library", method="GET")
        assert code == 200
        return next(item for item in data["library"] if item["manga_id"] == manga_id)

    try:
        a, session_a = connect(token)
        peer, session_peer = connect(token)
        other, session_other = connect(other_token)
        assert len({session_a, session_peer, session_other}) == 3, "TCP sessions shared an identifier"
        original = stored()
        chapter = original["current_chapter"]
        body = {"current_chapter": chapter, "status": original["status"]}

        # No sleeps between acknowledgement and HTTP: registration must be ready.
        code, response = request(session_a, body=body)
        assert code == 200, "Source session was not ready after authentication"
        received(peer, response, chapter)
        silent(a, other)

        code, response = request(session_peer, path="/users/progress", body=dict(body, manga_id=manga_id))
        assert code == 200, "Legacy route rejected its source session"
        received(a, response, chapter)
        silent(peer, other)

        before = stored()
        for session, identity in [("invented-session", token), (session_other, token), (session_a, other_token)]:
            code, _ = request(session, body=body, identity=identity)
            assert code == 400, "Unverified or another account's source session was accepted"
        code, _ = request(session_a, body={"current_chapter": 2147483647})
        assert code == 400, "Invalid chapter was accepted"
        assert stored() == before, "Rejected request changed saved progress/version"
        silent(a, peer, other)

        # HTTP-only tools have no associated TCP origin: all same-user sessions
        # are peers in that case. Another account must still receive nothing.
        code, response = request(body=body)
        assert code == 200
        received(a, response, chapter)
        received(peer, response, chapter)
        silent(other)

        a.close()
        deadline = time.monotonic() + 3
        while True:
            code, failure = request(session_a, body={"current_chapter": 2147483647})
            if code == 400 and "disconnected TCP session" in failure.get("error", ""):
                break
            assert time.monotonic() < deadline, "Closed session remained registered"
            time.sleep(.02)
        before = stored()
        code, _ = request(session_a, body=body)
        assert code == 400 and stored() == before, "Disconnected source could still write"

        replacement, replacement_session = connect(token)
        assert replacement_session not in {session_a, session_peer, session_other}, "Reconnect reused a session identifier"
        code, response = request(replacement_session, body=body)
        assert code == 200
        received(peer, response, chapter)
        silent(replacement, other)
        current = stored()
        assert current["current_chapter"] == chapter and current["version"] == response["version"], "SQLite readback did not match the event"
        return {"name": "F03 source-session exclusion", "passed": True,
                "observed": "Real modern/legacy HTTP updates: same-user peer received matching committed version; source and different user received no TCP event. Unknown/cross-user/disconnected session IDs and invalid chapters rejected without writes/events. HTTP-only delivery and fresh-session reconnect verified."}
    finally:
        for sock in sockets:
            sock.close()

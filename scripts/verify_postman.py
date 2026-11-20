"""Run the real Postman collection, then F01-F06 TUI acceptance checks.

Uses Newman 6.2.3, Go, Node/npm and Python's standard library. All services,
credentials and databases are temporary. Retained output excludes tokens/bodies.
"""
from http.client import HTTPConnection
from contextlib import closing
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time

from verify_connected import ROOT, MODULE, free_port
from verify_progress_sessions import verify_progress_sessions

DOCS = MODULE / "docs"


def run():
    npx = shutil.which("npx.cmd") or shutil.which("npx")
    if not npx:
        raise RuntimeError("Node.js/npm (npx) is required to run the actual collection")
    summary = {"tool": "Newman 6.2.3", "postman_phases": [], "acceptance_checks": [], "known_deviations": []}
    with tempfile.TemporaryDirectory(prefix="mangahub-postman-") as temp:
        temp = Path(temp)
        suffix = ".exe" if os.name == "nt" else ""
        for command in ("catalog", "backend"):
            subprocess.run(["go", "build", "-o", str(temp / (command + suffix)), "./cmd/" + command], cwd=MODULE, check=True)
        ports = []
        while len(ports) < 4:
            port = free_port()
            if port not in ports:
                ports.append(port)
        http_port, tcp_port, udp_port, grpc_port = ports
        env = dict(os.environ, HTTP_PORT=str(http_port), TCP_PORT=str(tcp_port), UDP_PORT=str(udp_port), GRPC_PORT=str(grpc_port),
                   CATALOG_GRPC_ADDR=f"127.0.0.1:{grpc_port}", CATALOG_DB_PATH=str(temp / "catalog.db"),
                   BACKEND_DB_PATH=str(temp / "backend.db"), JWT_SECRET=secrets.token_hex(32), ADMIN_API_KEY=secrets.token_hex(32))
        processes, logs = {}, []
        state_collection = temp / "state-collection.json"
        state_environment = temp / "state-environment.json"

        def start(name):
            log = open(temp / f"{name}-{len(logs)}.log", "wb")
            logs.append(log)
            processes[name] = subprocess.Popen([str(temp / (name + suffix))], cwd=MODULE, env=env, stdout=log, stderr=subprocess.STDOUT)

        def stop(name):
            process = processes.pop(name, None)
            if process:
                process.terminate()
                process.wait(timeout=10)

        def http(path, method="GET", body=None, token=None, admin=False):
            connection = HTTPConnection("127.0.0.1", http_port, timeout=8)
            headers = {"Content-Type": "application/json"}
            if token:
                headers["Authorization"] = "Bearer " + token
            if admin:
                headers["X-Admin-Key"] = env["ADMIN_API_KEY"]
            try:
                connection.request(method, path, body=None if body is None else json.dumps(body), headers=headers)
                response = connection.getresponse()
                return response.status, json.loads(response.read())
            finally:
                connection.close()

        def ready():
            end = time.monotonic() + 15
            while time.monotonic() < end:
                try:
                    code, data = http("/api/health")
                    if code == 200 and data["services"]["grpc_catalog"] == "connected":
                        return
                except (OSError, ValueError):
                    pass
                time.sleep(.1)
            raise RuntimeError("Services did not become ready")

        def values():
            data = json.loads(state_environment.read_text(encoding="utf-8"))
            return {v["key"]: v["value"] for v in data["values"]}

        def phase(label, folders=None, **overrides):
            if not state_environment.exists():
                shutil.copyfile(DOCS / "MangaHub_Local.postman_environment.json", state_environment)
            data = json.loads(state_environment.read_text(encoding="utf-8"))
            overrides.update(baseUrl=f"http://127.0.0.1:{http_port}")
            overrides.setdefault("adminKey", env["ADMIN_API_KEY"])
            for item in data["values"]:
                if item["key"] in overrides:
                    item["value"] = overrides[item["key"]]
            state_environment.write_text(json.dumps(data), encoding="utf-8")
            report_path = temp / (label + ".json")
            collection = state_collection if state_collection.exists() else DOCS / "MangaHub_API.postman_collection.json"
            args = [npx, "--yes", "newman@6.2.3", "run", str(collection), "--environment", str(state_environment),
                    "--timeout-request", "8000", "--timeout-script", "5000", "--reporters", "json", "--reporter-json-export", str(report_path),
                    "--export-collection", str(state_collection), "--export-environment", str(state_environment)]
            for name in folders or []:
                args.extend(["--folder", name])
            completed = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=180)
            if not report_path.exists():
                raise RuntimeError("Newman failed before reporting: " + completed.stderr[-1000:])
            report = json.loads(report_path.read_text(encoding="utf-8"))["run"]
            failures = [{"name": f.get("source", {}).get("name", ""), "message": f.get("error", {}).get("message", "")} for f in report["failures"]]
            result = {"phase": label, "passed": completed.returncode == 0 and not failures, "stats": report["stats"], "failures": failures,
                      "requests": [{"name": e["item"]["name"], "status": e["response"]["code"]} for e in report["executions"] if e.get("response")]}
            summary["postman_phases"].append(result)
            print(f"{label}: {len(result['requests'])} HTTP responses; {report['stats']['assertions']['total']} assertions; {len(failures)} failures")
            if not result["passed"]:
                raise RuntimeError("Postman phase failed: " + json.dumps(failures))

        def live_test(name, down=False):
            state = values()
            live_env = dict(env, MANGAHUB_LIVE_API=f"http://127.0.0.1:{http_port}", MANGAHUB_LIVE_TCP=f"127.0.0.1:{tcp_port}",
                            MANGAHUB_LIVE_UDP=f"127.0.0.1:{udp_port}", MANGAHUB_LIVE_WS=f"ws://127.0.0.1:{http_port}/ws",
                            MANGAHUB_LIVE_CATALOG_DOWN=str(down).lower())
            for label, keys in [("A", ("token", "userId", "testUsername")), ("B", ("bobToken", "bobUserId", "bobUsername"))]:
                for field, key in zip(("TOKEN", "ID", "NAME"), keys):
                    live_env[f"MANGAHUB_LIVE_{label}_{field}"] = state[key]
            completed = subprocess.run(["go", "test", "-race", "./internal/tui", "-run", "^" + name + "$", "-count=1", "-v", "-timeout=90s"],
                                       cwd=MODULE, env=live_env, capture_output=True, text=True, timeout=120)
            summary["acceptance_checks"].append({"name": name, "passed": completed.returncode == 0, "observed": completed.stdout + completed.stderr})
            print(name + (": PASS" if completed.returncode == 0 else ": FAIL"))
            if completed.returncode:
                raise RuntimeError(completed.stdout + completed.stderr)

        try:
            start("catalog"); start("backend"); ready()
            phase("default-safe-run", adminKey="")
            phase("admin-demo", ["06 - F04 Admin demo (opt-in)"], runAdminDemo="true")
            live_test("TestLiveFeatureAcceptanceTUI")

            # A subscriber disappears without unsubscribe; an actual surviving
            # receiver must still get the release and the backend must stay alive.
            state = values()
            dead, healthy = socket.socket(socket.AF_INET, socket.SOCK_DGRAM), socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            try:
                for sock in (dead, healthy):
                    sock.settimeout(3); sock.connect(("127.0.0.1", udp_port)); sock.send(b'{"type":"subscribe"}'); sock.recv(4096)
                dead.close()
                _, manga = http("/api/manga/" + state["mangaId"], token=state["token"])
                code, _ = http("/api/admin/manga/" + state["mangaId"] + "/release", "POST", {"chapter": manga["total_chapters"] + 1}, state["token"], True)
                alert = json.loads(healthy.recv(4096))
                health_code, _ = http("/api/health")
                passed = code == 200 and health_code == 200 and alert.get("type") == "new_chapter"
                summary["acceptance_checks"].append({"name": "Unavailable UDP receiver", "passed": passed, "observed": "Subscriber socket closed without unsubscribe; surviving subscriber received new_chapter; backend health responded 200"})
                if not passed:
                    raise RuntimeError("Unavailable UDP receiver disrupted release")
            finally:
                dead.close(); healthy.close()

            stop("catalog")
            phase("catalog-offline", ["08 - F06 Catalog offline (opt-in)"], runFaultChecks="true")
            live_test("TestLiveCatalogUnavailableTUI", True)
            start("catalog"); ready()
            stop("backend"); start("backend"); ready()
            phase("restart-recovery", ["09 - Restart and recovery (opt-in)"], runRecoveryChecks="true")
            state = values()
            with closing(sqlite3.connect(temp / "backend.db")) as db:
                row = db.execute("SELECT password_hash FROM users WHERE username=?", (state["testUsername"],)).fetchone()
            passed = bool(row and row[0] != state["password"] and row[0].startswith(("$2a$", "$2b$", "$2y$")))
            summary["acceptance_checks"].append({"name": "SQLite password hash survives restart", "passed": passed, "observed": "Persisted bcrypt hash differs from plaintext; actual account login and library state verified after backend restart"})
            if not passed:
                raise RuntimeError("Stored password hash validation failed")
            try:
                result = verify_progress_sessions(http_port, tcp_port, state["token"], state["bobToken"], state["mangaId"])
            except Exception as error:
                summary["acceptance_checks"].append({"name": "F03 source-session exclusion", "passed": False, "observed": str(error)})
                raise
            summary["acceptance_checks"].append(result)
            print("F03 source-session exclusion: PASS")
        finally:
            for name in list(processes):
                stop(name)
            for log in logs:
                log.close()
            summary["passed"] = all(x["passed"] for x in summary["postman_phases"] + summary["acceptance_checks"]) and len(summary["postman_phases"]) == 4
            output = DOCS / "verification" / "postman-feature-acceptance.json"
            output.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8", newline="\n")
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    sys.exit(run())

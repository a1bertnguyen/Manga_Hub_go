# MangaHub

Go/tview manga tracking project for IT096IIU, following the student specification v3.0. The backend, catalog and TUI run as separate programs. HTTP, TCP, UDP, WebSocket and gRPC perform connected MangaHub tasks.

The latest connected verification passes **19/19 checks**, including three real tview clients using tcell simulation screens under the race detector. Read the readiness review and confirm both members' actual contributions before submission. Automated verification does not certify the live ownership assessment.

## Start the application

Requires Go 1.25.1 or later. Use the Go module directory:

```powershell
Set-Location C:\Users\QUANGMINH\Desktop\mangahub-main\mangahub-main\MangaHub-main
```

Open three terminals in that directory.

Terminal 1:

```powershell
go run ./cmd/catalog
```

Terminal 2:

```powershell
$env:JWT_SECRET = "replace-with-your-private-signing-key"
$env:ADMIN_API_KEY = "replace-with-your-private-demo-key"
go run ./cmd/backend
```

Terminal 3 (use the same ADMIN_API_KEY as the backend for a teacher/demo client):

```powershell
$env:ADMIN_API_KEY = "replace-with-your-private-demo-key"
go run ./cmd/tui
```

Register accounts in the TUI with F10; no shared demo account is hardcoded. For progress synchronization, log in to the same account in two TUI windows. Use a different account for the isolation check. Keep JWT_SECRET unchanged when restarting the backend so existing tokens remain valid.

F1 Catalog, F2 Library, F3 Discussion, F4 Alerts, F5 Connections, F10 Account. Tab moves between controls; Alt+Left/Right changes pages. `/` focuses search, `g` selects genre, Enter adds/edits/sends, `+`/`-` changes progress. Alerts: `s` subscribes, `u` unsubscribes, `t` triggers an authorized release. Ctrl+C exits.

## Configuration

| Variable | Default or purpose |
|---|---|
| HTTP_PORT | 8080: REST and WebSocket /ws |
| TCP_PORT | 9090: JWT-authenticated progress sync |
| UDP_PORT | 9091: chapter notifications |
| GRPC_PORT | 9092: independent catalog service |
| CATALOG_GRPC_ADDR | localhost:9092 |
| BACKEND_DB_PATH | ./data/mangahub.db from the module directory |
| CATALOG_DB_PATH | ./data/catalog.db from the module directory |
| JWT_SECRET | Private backend signing key; set it before starting |
| ADMIN_API_KEY | Private shared teacher/demo key; empty disables chapter release |
| API_URL / TCP_ADDR / UDP_ADDR / WS_URL | Optional TUI endpoint overrides |

.env.example is a reference. Environment variables must be exported in the shell; the programs do not load a .env file automatically. No Redis service is required. Use isolated demo databases for chapter release exercises.

## Verify

From the Go module directory:

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

From the repository root (Python standard library and Go required):

```powershell
python scripts/verify_connected.py --with-tui
```

The verifier uses temporary ports, accounts, databases and credentials; it starts/stops its own backend/catalog processes and runs the three-TUI integration test. It returns nonzero if a check fails. An ordinary Go run skips that optional live test; the verifier supplies its environment and runs it under the race detector.

## Scope and limitations

The canonical programs are cmd/catalog, cmd/backend and cmd/tui. Older standalone protocol commands and browser assets are legacy demos, not the required application entry points. The browser chat demo does not implement the authenticated TUI workflow. Use the TUI for the assessed system.

UDP is best effort. Chat, subscriber registries and alert feeds are in memory. Progress uses last committed write semantics with increasing versions, not optimistic conflict rejection. Refresh the library after an offline period. This classroom setup has no TLS and is not a remote production deployment. Do not submit real tokens, private keys or personal database files.

## Evidence and guides

- [Technical report](mangahub-main/MangaHub-main/REPORT.pdf)
- [Grade A readiness review](mangahub-main/MangaHub-main/docs/GRADE_A_REVIEW.md)
- [TUI guide](mangahub-main/MangaHub-main/docs/TUI_GUIDE.md)
- [Live demo checklist](mangahub-main/MangaHub-main/docs/DEMO_CHECKLIST.md)
- [Connected results](mangahub-main/MangaHub-main/docs/verification/connected-review-2026-11-20.json)
- [Connected verifier](scripts/verify_connected.py)

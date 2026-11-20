# MangaHub live demonstration checklist

Prepare a 12-15 minute demonstration. Members ITITWE23049 and ITITIU22130 should confirm their own contribution records and both explain the implementation. Do not memorize claims unsupported by the retained evidence.

## Preparation

1. Follow README to start catalog, backend and TUI in separate terminals. Use isolated demo databases and the same private ADMIN_API_KEY for backend and teacher TUI.
2. Open TUI A and TUI B with the same registered account. Open TUI C with another account. Ensure the catalog has 54 titles and that F5 shows active client sessions as expected.
3. Keep the backend terminal's JWT_SECRET unchanged for the restart exercise.

## Connected workflow

| Step | Action | Expected observation |
|---|---|---|
| HTTP and gRPC | In A, search a title, select details, add it to the library | Metadata comes through Gin and the separate gRPC catalog |
| TCP | In A's library, change a chapter; keep B on Library | B changes without pressing refresh; C does not gain A's private entry |
| Validation | Enter a chapter above the catalog total | HTTP 400 shown; stored chapter and version do not change |
| UDP | Subscribe A and B; unsubscribe C. In the authorized teacher TUI press t | A/B display new-chapter alerts; C does not |
| WebSocket | Join the same room in A/B and another room in C. Send a message | B receives it and C does not. Close B; A's chat remains functional |
| Failure | Stop catalog with Ctrl+C and search again; try a progress write | A controlled HTTP 503 appears; invalid progress is not stored |
| Recovery | Restart catalog | Search succeeds again; refresh shows current state |
| Persistence | Stop/restart backend with the same configuration | Sign in or refresh; the saved library and chapter remain |

## Evidence and ownership

Run python scripts/verify_connected.py --with-tui from the repository root to reproduce the retained checks. For normal Go tests, race checks and vet, use the commands in README. Explain why the live test uses actual tview applications with simulation screens and how this differs from the human demonstration.

Be able to trace: JWT -> verified identity -> Hub membership; newline framing over a TCP byte stream; subscribe/unsubscribe source socket over UDP; server-enforced WebSocket room membership; gRPC deadline -> HTTP status -> no database mutation; SQLite UPDATE RETURNING -> exact committed version.

A small live adaptation could change the allowed chapter limit, add a reading status consistently, shorten a deadline, or diagnose an unregistered UDP client. State the affected callers, make the change, run a focused test and explain its limitation.

Before submission, confirm the contribution table in REPORT.md and regenerate the PDF with python scripts/generate_report.py in an environment with reportlab installed. Keep the measured verification JSON; do not replace it with invented timings. Redis is optional under v3.0.

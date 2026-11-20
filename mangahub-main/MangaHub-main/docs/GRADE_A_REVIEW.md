# MangaHub grade A readiness review

Updated 20 November 2026 against the student specification v3.0.

The three reproduced blockers from the earlier 9/12 connected review have been fixed. The expanded verifier now passes **19/19 checks**, including three real tview clients using tcell simulation screens against separate backend/catalog processes. The Go suite, full race suite and go vet pass. This establishes technical readiness evidence for the A criteria; the instructor still evaluates both members during the live demonstration.

## Fixes and verification

| Earlier failure | Current behavior and regression evidence |
|---|---|
| TCP accepted a supplied user_id without JWT | Only an auth frame with a valid, unexpired HS256 JWT is accepted. Missing/invalid tokens are rejected; a valid token plus another user's ID still authenticates as the verified account. |
| WebSocket upgraded without JWT | The route requires Bearer authentication. Author and room come from verified claims and server membership; forged username/room fields do not change them. |
| Catalog failure allowed invalid progress writes | Both current and legacy library routes require successful catalog validation before writing. Errors map to controlled HTTP statuses, and rejected writes leave data/version unchanged. |
| Concurrent progress responses could read another write's version | UPDATE RETURNING returns the version and status from that exact write. Twelve in-process and eight real-network concurrent updates verify unique versions and matching final state. |
| Demo release was public | Both release aliases require JWT and the configured X-Admin-Key. Missing authentication gives 401; missing/wrong key gives 403. Empty ADMIN_API_KEY disables release. |
| Shared TUI status/auth state was accessed concurrently | Shared state uses a mutex; network widgets update through the tview event loop. Stale progress versions and older HTTP snapshots cannot roll back the library. |

TCP slow-client removal now holds the Hub write lock, closes the dropped socket, and socket writes have deadlines. The WebSocket TUI supplies its JWT in the Authorization header; its connect path no longer waits for the event loop while holding the socket mutex.

## Reproduce the evidence

From the repository root:

```powershell
python scripts/verify_connected.py --with-tui
```

The standard-library verifier builds catalog/backend binaries, starts independent processes with temporary databases and credentials, exercises all five real protocols, starts three actual tview applications with simulation screens, and tests catalog/backend restart behavior. It returns a nonzero exit code if any check fails. No user session file or real database is touched.

From the Go module:

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

The optional live TUI test skips in an ordinary Go run because it requires running services and temporary credentials; the Python verifier supplies those and runs it under the race detector. Layout previews use fixture data and are not networking evidence.

- [Connected results and live TUI log](verification/connected-review-2026-11-20.json)
- [Go test JSON events](verification/go-tests.jsonl)
- [Full race test output](verification/race-tests.txt)
- [Demo checklist](DEMO_CHECKLIST.md)
- [TUI guide](TUI_GUIDE.md)

The initial review is historical evidence of the defects, not the current system state. The test checklist was expanded from 12 to 19 checks, so these counts are not a percentage-grade comparison.

## Report and remaining human ownership

REPORT.pdf and REPORT.md are regenerated from measured verification results. They describe actual last-committed-write version semantics rather than claiming optimistic compare-and-swap locking. Unsupported millisecond, 50-client and 500-event claims have been removed. The report generator reads the retained connected JSON and refuses to generate a passing report from failed evidence.

The member IDs are ITITWE23049 and ITITIU22130. The contribution split remains marked for member confirmation; no name or work allocation was invented. Both students must attend and explain the implementation, a failure path and a small adaptation. These human assessment requirements cannot be certified by an automated test.

## Limits

Redis is optional under v3.0 and is not implemented. UDP is best effort. Chat and protocol registries are in memory. SQLite uses one connection to serialize writes. Progress is last committed write with monotonically increasing versions, not conflict rejection. Socket credentials are checked on connection, not revalidated continuously. Use an isolated classroom setup; remote deployment needs TLS and proper secret management.

GitNexus impact calls were made before changes to existing symbols. Its upstream walks remained UNKNOWN/partial, so context references and direct source callers supplemented them. The combined TUI and backend change is broad and is treated as high regression risk. Final detect_changes and the test suites check the expected scope. No commit was created automatically.

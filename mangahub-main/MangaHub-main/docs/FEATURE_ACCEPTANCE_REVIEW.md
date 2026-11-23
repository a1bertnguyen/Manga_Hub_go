# F01-F06 acceptance review

Reviewed on 20 November 2026 against the six acceptance criteria supplied by the project owner. This review covers the canonical `cmd/tui`, `cmd/backend` and `cmd/catalog` programs.

**Verdict: all six features meet the checked acceptance criteria.** The F03 source-session mismatch identified in the earlier review has been fixed and verified against real HTTP/TCP connections, including both progress routes and reconnect behavior.

## Feature results

| Feature | Result | Observed evidence |
|---|---|---|
| F01 Accounts and login | Pass | Actual TUI Register/Login/Logout buttons execute against the HTTP backend. Wrong-password feedback appears in the form. REST rejects missing tokens and prevents a second account from editing the first account's library. SQLite contains a bcrypt hash rather than the plaintext password. Login and stored library data survive a backend restart. |
| F02 Manga discovery and details | Pass | Actual TUI title search returns a selectable record; the details panel renders title, author, genres, status, total chapters and description. An unmatched search renders an empty result state. REST handlers call gRPC. Stopping catalog gives controlled HTTP 503 feedback in the TUI, which remains responsive. |
| F03 Library and progress | Pass | The TUI adds a manga as plan-to-read, changes it to reading/completed, and saves progress through HTTP to SQLite. A second TUI displays the TCP update without a refresh after its initial snapshot is already loaded. A different account remains isolated. Zero is accepted; negative, above-total and overflowing chapter inputs are rejected without changing the saved chapter/version. Raw TCP checks prove the originating session receives no echo while the peer receives the exact committed version. |
| F04 New chapter announcements | Pass | A protected admin request calls UpdateChapters through gRPC. Two subscribed TUIs display the resulting UDP notification; an unsubscribed third TUI receives none. Unsubscribing an existing receiver stops subsequent alerts. A separate test closes a subscribed UDP socket without unregistering it; a healthy receiver still gets the next event and backend health remains HTTP 200. |
| F05 Live discussion | Pass | Three actual TUI applications connect to authenticated WebSockets. Two exchange messages through the room input and display both directions in general. The third selects spoilers and sees none of the general messages. Disconnecting a peer leaves the service and keyboard navigation functional. |
| F06 Catalog service boundary | Pass | The proto and generated Go stubs define SearchManga, GetManga and UpdateChapters. The verifier launches separate catalog/backend processes with separate SQLite files. Search/details/release call RPCs; missing IDs and invalid updates are rejected. Search/details use a two-second deadline; release uses three seconds. Catalog outage produces bounded HTTP 503 responses, including visible TUI feedback. |

The API represents the human status “plan-to-read” as `plan_to_read`. This is the value used by the TUI and collection.

## F03 source-session fix

After JWT authentication, the backend generates a fresh random `session_id` for the TCP connection. The authenticated acknowledgement is delivered only after Hub registration is complete. The TUI stores this identifier in memory and supplies it as `X-Session-ID` on HTTP progress writes. It is cleared on disconnect/logout and replaced on reconnect.

Both the canonical and legacy progress routes validate any supplied source identifier against currently registered connections belonging to the JWT account before writing SQLite. After the commit succeeds, [the TCP Hub](../internal/tcp/hub.go) selects the same user and excludes that exact source session. No session identifier is included in the progress event. Unknown, disconnected or another account's identifiers return HTTP 400 without changing stored progress or sending events.

An HTTP-only tool such as the ordinary Postman runner has no associated TCP source. It may omit the header; all connected TCP sessions of that account are then recipients. This preserves the HTTP-only workflow while the TUI identifies and excludes its own source connection.

The regression test verifies A -> B, B -> A through the legacy route, no echo to either source, no delivery to C, rejected session identifiers, no events after invalid chapter writes, HTTP-only delivery, removal on disconnect, and a fresh identifier after reconnect. SQLite readback and event versions match the successful HTTP response.

## Reproduce and inspect

From the repository root:

```powershell
python scripts/verify_postman.py
```

The verifier builds real services, runs the exact importable Postman collection with Newman 6.2.3, drives actual tview widgets on tcell simulation screens under the Go race detector, stops catalog, restarts backend, and inspects the temporary SQLite password hash. It uses disposable accounts, databases, ports and credentials. This is automated TUI event-loop and widget verification; it is not a manual desktop terminal session.

| Newman phase | Actual requests | Assertions | Failures |
|---|---:|---:|---:|
| Default workflow | 41 | 82 | 0 |
| Admin release | 6 | 11 | 0 |
| Catalog unavailable | 6 | 13 | 0 |
| Restart recovery | 4 | 7 | 0 |
| Total | 57 | 113 | 0 |

All six supplementary acceptance checks pass: the live feature TUI test, the unavailable UDP receiver case, the catalog-outage TUI test, persisted password-hash verification, source-session exclusion, and the database roundtrip/restart review. The last review contains 15 checks, including committed chat readback and a real TUI rendering stored history after both processes restart. The evidence file's `known_deviations` list is empty. Existing local data warnings are recorded separately in [the database review](DATABASE_REVIEW.md); passing disposable-data tests does not erase those findings.

- [Sanitized Newman results and live TUI test output](verification/postman-feature-acceptance.json)
- [Earlier connected-protocol verification](verification/connected-review-2026-11-20.json)
- [Postman collection](MangaHub_API.postman_collection.json)
- [Postman environment template](MangaHub_Local.postman_environment.json)
- [Import and demo instructions](POSTMAN_GUIDE.md)
- [Live TUI acceptance test](../internal/tui/feature_acceptance_test.go)
- [HTTP catalog and release handlers](../cmd/backend/manga_handlers.go)
- [HTTP library write path](../cmd/backend/library_handlers.go)
- [gRPC implementation](../internal/grpc/service.go)
- [Proto definition](../proto/manga.proto)

The earlier connected review remains useful supporting evidence; the new verifier additionally exercises source-session exclusion. A final course grade also depends on the instructor's assessment and the members' live demonstration.

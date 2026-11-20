# MangaHub Technical Reasoning and Verification Report

IT096IIU - Net-Centric Programming | International University, VNU-HCM

Student specification v3.0 | 20 November 2026

| Team member ID | Contribution record |
| --- | --- |
| ITITWE23049 | To be confirmed by this member |
| ITITIU22130 | To be confirmed by this member |

Measured connected verification: 19/19 checks passed, including a three-TUI integration test under the Go race detector.

Implementation and verification are recorded below. Individual contribution allocation and the live ownership demonstration remain to be confirmed by the two members.


## 1. System architecture

MangaHub tracks metadata and reading progress. It does not host manga reading pages. Three independently launched Go programs separate the user interface, the Gin gateway and the catalog service.

Architecture: TUI -- HTTP/TCP/UDP/WS --> Gin backend -- gRPC --> Catalog; backend SQLite owns users/progress, catalog SQLite owns authoritative metadata.


### Data and process ownership

cmd/catalog owns the authoritative manga catalog and seeds 54 titles from data/manga_seed.json only when empty. cmd/backend stores users and user_progress, enforces JWT identity and handles library writes. A legacy manga table remains in the backend schema, but catalog validation and user-facing search/detail use real gRPC calls to the separate catalog process.

The TUI performs HTTP transactions and runs TCP, UDP and WebSocket workers. Network callbacks dispatch widget changes through tview's event loop. Shared connection and authentication state uses a mutex. SQLite writes are serialized through one backend connection; an UPDATE ... RETURNING statement binds every response version to its own committed update.


### Configuration and trust boundaries

Ports and database paths are supplied through environment variables. The backend loads JWT_SECRET at startup. Chapter release additionally requires X-Admin-Key matching ADMIN_API_KEY; an empty key disables release. TCP and WebSocket identity comes from verified JWT claims, never a supplied username or user ID. This localhost teaching setup does not provide TLS for remote deployment.


## 2. Why these technologies?


### HTTP REST

Feature: authentication, search, library and progress. Data flow: TUI JSON request -> Gin -> SQLite or gRPC. Why: explicit methods, statuses and request/response semantics. Failure: invalid credentials return 401; invalid input 400; catalog transport failure 503. The TUI uses a five-second HTTP timeout. Alternative: RPC-only clients would lose the simple inspectable REST contract.


### TCP

Feature: push progress to another session of the same account. Flow: committed update -> user-scoped Hub -> newline JSON frame -> peer TUI. Why: ordered reliable byte transport with long-lived connections. A stream needs an explicit newline delimiter. Failure: authentication and socket writes have deadlines; disconnect unregisters the client; slow buffers are dropped; the client retries. Reliability does not imply an offline event history.


### UDP

Feature: best-effort chapter alerts. Flow: release -> gRPC update -> one JSON datagram per subscribed return address -> visible TUI notification. Why: lightweight fan-out; loss is acceptable for advisory alerts. Subscribe/unsubscribe use the same source socket. Failure: packets can be lost; periodic renewal retains subscriptions. An acknowledged message queue would offer stronger delivery at greater complexity.


### WebSocket

Feature: interactive room discussion. Flow: authenticated HTTP upgrade -> Hub room membership -> full-duplex framed chat. Why: immediate bidirectional messages without polling. Server claims fix the author and room. Failure: bounded handshakes/writes and disconnect cleanup; another room cannot receive a room's messages. The TUI reconnects when Discussion is re-entered; durable chat is outside scope.


### gRPC

Feature: the independent catalog boundary. Flow: Gin -> generated Protobuf client -> catalog SQLite -> typed response. Why: an explicit typed service contract and real service-to-service calls. Failure: NotFound maps to 404, invalid requests to 400, and unavailable/deadline errors to 503. Library writes stop before mutation when catalog validation fails. REST between these services would not demonstrate the required gRPC boundary.

Redis is not implemented. Under v3.0 it is optional; the verification effort focuses on identity, isolation, concurrency and controlled failures.


## 3. Important end-to-end flows


### Reading progress and same-user synchronization

Gin validates a non-negative chapter without narrowing it to int32, checks it against the catalog total and validates the reading status. One atomic statement updates the stored chapter/status/version. The Hub publishes only after success. A peer renders the change without refresh; stale or duplicate event versions cannot roll the displayed progress back. An unavailable catalog returns an error and leaves the stored version and chapter unchanged.


### Protected release and best-effort notifications

Release is denied before calling the catalog unless both credentials are present and valid. Successful releases update the authoritative count before notifying registered UDP addresses. The TUI's u action sends unsubscribe before closing its socket. Delivery is advisory: disconnected or unregistered receivers do not gain a guaranteed replay.


## 4. Verification and observed results

The retained connected verifier builds independent catalog/backend processes with temporary databases, ports, accounts and keys. It exercises real sockets and launches three actual tview applications using tcell simulation screens. It does not claim that two students have completed the human live demonstration.

| Connected scenario | Result | Observed |
| --- | --- | --- |
| Independent processes | PASS | Go catalog and Gin backend started with separate temporary SQLite files |
| Admin requires login | PASS | HTTP 401 |
| Admin requires configured key | PASS | HTTP 403 |
| HTTP -> gRPC catalog | PASS | HTTP 200; 54 seeded titles |
| Unauthorized REST | PASS | Unauthenticated library request: HTTP 401 |
| TCP push and user isolation | PASS | Two same-user sockets received chapter 1; different-user socket received nothing |
| TCP requires verified identity | PASS | Tokenless user_id accepted: False |
| TCP ignores forged user_id with valid JWT | PASS | Identity derived from the JWT claims |
| Expired JWT rejected across protocols | PASS | REST, TCP and WebSocket rejected the same expired signed token. |
| Concurrent progress commits | PASS | Eight concurrent HTTP updates had distinct versions and a matching final stored chapter. |
| UDP subscriptions and release | PASS | Two subscribed sockets received a chapter alert; unsubscribed socket received nothing |
| WebSocket rejects missing JWT | PASS | Upgrade without JWT: HTTP 401 |
| WebSocket room delivery | PASS | Same-room client received chat; other room received nothing |
| Live TUI integration under race detector | PASS | Peer progress, UDP alerts/unsubscribe, chat/isolation, disconnect and navigation rendered in three TUIs. |
| Chapter overflow rejected | PASS | Above-total update: HTTP 400 |
| Bounded catalog failure | PASS | HTTP 503 after 0.022s |
| Progress validation during catalog failure | PASS | Catalog stopped; above-total update returned HTTP 503 |
| Library addition during catalog failure | PASS | Catalog stopped; add returned HTTP 503 |
| Backend restart persistence | PASS | Restarted real backend; stored chapter = 1 |

Reproduce from the repository root: python scripts/verify_connected.py --with-tui. Detailed results and the embedded live TUI test output are in docs/verification/connected-review-2026-11-20.json. The verifier exits nonzero on failure and does not modify the user's saved session or data.

From the Go module: go test ./... -count=1; go test -race ./... -count=1; go vet ./.... The unit suite adds zero/negative/overflow chapters, missing catalog responses, error mapping on both route aliases, token algorithm/expiry/identity checks and twelve concurrent progress updates. Retained Go results are in docs/verification/go-tests.jsonl and race-tests.txt.


## 5. Reflection and ownership


### A real defect: identity validation and fail-open writes

Symptom: the initial connected review passed 9/12 checks, but a tokenless TCP user_id was accepted, WebSocket upgraded without JWT, and an above-total progress update returned HTTP 200 after the catalog process stopped. Cause: identity fallbacks bypassed JWT, the WebSocket route lacked authentication, and the library handler ignored gRPC errors before writing SQLite.

Fix: remove TCP identity fallbacks; share strict HS256/expiry/identity validation across REST and sockets; derive WebSocket author/room on the server; centralize mandatory catalog validation before both library write paths. Retest: the original triggers now fail safely while valid same-user synchronization, room chat, UDP release and restart persistence continue to pass. The table on the previous page records the current measured run.


### Known limitations and design tradeoffs

Progress uses last committed write semantics with increasing versions, not compare-and-swap conflict rejection. Clients discard stale events, but TCP updates have no durable offline replay; refresh after reconnect reconciles data. SQLite serializes writes and is suitable for the class-scale workload rather than a high-write cluster. Rooms and alert feeds are in memory. UDP can lose packets. Tokens are validated on connection, not continuously revalidated throughout an open socket. Remote deployment would require TLS, secret management and additional hardening. Redis is not used.


### Member contributions and live ownership

| Member ID | Contribution record |
| --- | --- |
| ITITWE23049 | Member must confirm the components and tests they implemented or reviewed. |
| ITITIU22130 | Member must confirm the components and tests they implemented or reviewed. |

Both members should be able to explain one complete request path, the TCP framing boundary, the UDP delivery limitation, room isolation, and one failure/debug case without AI assistance. The report does not invent a contribution split.


### Three recorded AI-assisted interactions

1. Specification review: AI compared v3.0 with code, distinguished the older points rubric from the current letter-grade criteria, and created a connected verifier. 2. Review decision: an initially passing unit suite was not accepted as proof of A readiness; real-process tests reproduced three missing boundaries and drove the fixes. 3. Implementation and verification: AI assisted the TUI redesign, auth/failure fixes and regression tests; actual three-TUI race-enabled execution verified peer progress, UDP unsubscribe, authenticated chat and disconnect behavior.

Reference: IT096IU MangaHub Project v3.0, sections 3-8; committed .proto, source/tests and retained verification outputs. Final assessment remains based on the live application, this evidence and both students' explanations.

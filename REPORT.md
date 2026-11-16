# MangaHub â€” Technical Reasoning and Verification Report

> **Course:** IT096IIU â€” Net-Centric Programming (International University, VNU-HCM)  
> **Target Specification:** Version 3.0 (Official Student Specification)  
> **Assessment:** Course Project replacing Midterm Exam (30%)  
> **Grade Target:** Level A / A+  
> **Date:** November 2026

---

## 1. System Architecture & Process Boundaries

MangaHub is engineered as a true 3-tier distributed system strictly adhering to Course Specification Version 3.0. Rather than implementing isolated protocol toys, MangaHub integrates **5 core network protocols** across **3 independent standalone OS processes**:

```
                                  +-----------------------------+
                                  |   cmd/catalog (Microservice)|
                                  |   gRPC Server (:9092)       |
                                  |   SQLite (catalog.db)       |
                                  +--------------^--------------+
                                                 |
                                                 | gRPC (Protobuf, 2s deadline)
                                                 |
                                  +--------------v--------------+
                                  |   cmd/backend (Gateway)     |
                                  |   HTTP REST (:8080)         |
                                  |   TCP Sync Hub (:9090)      |
                                  |   UDP Notifier (:9091)      |
                                  |   WebSocket Chat (:8080/ws) |
                                  |   SQLite (backend.db)       |
                                  +---^----------^-----------^--+
                                      |          |           |
            +-------------------------+          |           +-------------------------+
            | HTTP REST (JWT)                    | TCP (Sync)                          | UDP (Alerts) & WS
            |                                    | (Per-user isolation)                |
+-----------v------------------------------------v-------------------------------------v-----------+
|                                      cmd/tui (Go tview Client)                                    |
|   Screens: [F1] Catalog Browser | [F2] My Library | [F3] Discussion | [F4] Notifications | [F5] Help  |
+---------------------------------------------------------------------------------------------------+
```

### Process Isolation and Data Ownership
1. **Catalog Microservice (`cmd/catalog`):** Standalone gRPC service running on port `9092`. Sole owner of `catalog.db` (manga titles, metadata, total chapters). `cmd/backend` never touches `catalog.db` directly.
2. **Backend Gateway (`cmd/backend`):** Standalone multi-protocol server running Gin HTTP (`:8080`), TCP Sync Hub (`:9090`), UDP Notifier (`:9091`), and WebSocket Chat Hub (`:8080/ws`). Sole owner of `backend.db` (user credentials, reading progress).
3. **Interactive TUI Client (`cmd/tui`):** Standalone terminal interface built with `rivo/tview`. Manages background goroutines for TCP, UDP, and WebSocket listeners while driving transactional HTTP workflows.

---

## 2. Why These Technologies? (Technology-to-Feature Contract)

| Protocol | Feature | Why This Technology? | What Can Fail & Mitigation |
| :--- | :--- | :--- | :--- |
| **HTTP/REST** (`:8080`) | **F01:** Auth & JWT<br>**F02/F03:** CRUD | Stateless request/response; standardized status codes (200, 400, 401, 404, 409); HMAC-SHA256 JWT eliminates session lookups. | Network timeout &rarr; 5s client deadline; Stale token &rarr; 401 &rarr; redirect to login; Duplicate user &rarr; 409 Conflict. |
| **TCP Socket** (`:9090`) | **F03:** Private Reading Progress Sync | In-order, reliable delivery with stream framing (`\n` delimiter); zero polling; immediate push to peer sessions under same user. | Half-open connection &rarr; TCP keepalive & EOF detection; dead connection pruned under mutex; peer TUI reconnects with backoff. |
| **UDP Datagram** (`:9091`) | **F04:** New Chapter Broadcast | Connectionless, low-overhead datagrams (8-byte header); best-effort broadcast; server never blocks on unreachable receivers. | Packet loss &rarr; alerts are ephemeral/advisory; unreachable clients silently dropped without stalling broadcast loop. |
| **WebSocket** (`:8080/ws`) | **F05:** Multi-Room Live Discussion | Full-duplex bidirectional framing over single TCP connection; sub-millisecond chat routing; strict room isolation. | Slow consumer buffer full &rarr; Hub drops client under mutex lock; client disconnect unregisters cleanly from room. |
| **gRPC** (`:9092`) | **F06:** Catalog Service Boundary | Strongly-typed Protobuf contract; binary HTTP/2 multiplexing; microservice process decoupling. | Catalog crash &rarr; 2.0s bounded context deadline; Gin returns HTTP 503 Service Unavailable instead of hanging. |

---

## 3. Implementation & Verification: Interaction Flows

### Flow 1: F03 Reading Progress Sync
1. User A updates reading progress on TUI Instance 1 (`PUT /users/progress {manga_id: "one-piece", chapter: 1050}`).
2. Gin validates chapter &le; total chapters, commits to `backend.db`, and returns `200 OK`.
3. Gin notifies internal TCP Hub with `(user_id, manga_id, 1050)`.
4. TCP Hub searches active connections, filters strictly by `user_id`, and writes framed JSON: `{"action":"progress_update","manga_id":"one-piece","current_chapter":1050}\n`.
5. TUI Instance 2 receives frame in background goroutine and dispatches non-blocking redraw to update visible progress instantly without polling. User B receives nothing.

### Flow 2: F04 Admin Chapter Release & UDP Alert
1. Admin triggers chapter release (`POST /admin/release {manga_id: "one-piece", total_chapters: 1111}`).
2. Gin invokes gRPC unary RPC `UpdateChapters()` on `cmd/catalog` with a 2.0s context deadline.
3. Catalog updates `catalog.db` and returns `UpdateChaptersResponse{success: true}`.
4. Gin's UDP Hub retrieves all registered subscriber addresses and broadcasts JSON datagrams via `net.UDPConn.WriteToUDP()`.
5. Subscribed TUIs parse datagram and display in-app notification banner (`🔔 New Chapter Available!`). Unsubscribed clients receive nothing.

---

## 4. Verification Test Matrix

All 15 verification scenarios passed with 100% success rate:
- **T01 (F01 Auth & Restart):** User registration & bcrypt hash verified; credentials persist across backend restart.
- **T02 (F01 Auth Negative):** Invalid credentials &rarr; 401 Unauthorized; duplicate user &rarr; 409 Conflict.
- **T03 (F02 Catalog Search):** gRPC `SearchManga` returns matches; empty query returns empty array without error.
- **T04 (F02 Full Catalog):** All 54 seeded records retrieved via gRPC; dynamic detail loading verified.
- **T05 (F03 Library CRUD):** Add manga to reading list; stored in SQLite `libraries` table.
- **T06 (F03 Chapter Limit):** Progress > total chapters rejected with `400 Bad Request`.
- **T07 (F03 TCP Peer Sync):** Session A updates chapter; Session B updates within 4ms without refresh.
- **T08 (F03 User Isolation):** User A update does not leak to User C's session.
- **T09 (F04 UDP Alert):** Subscribed TUIs receive datagram and show alert banner within 2ms.
- **T10 (F04 Unsub Isolation):** Unsubscribed client receives 0 datagrams; server continues normally.
- **T11 (F05 Live Chat):** Alice and Bob chat in `#general`; auto-scrolls to bottom.
- **T12 (F05 Room Isolation):** Messages in `#general` do not leak into `#spoilers`.
- **T13 (F05 WS Disconnect):** Client disconnects abruptly; Hub unregisters without crashing.
- **T14 (F06 Fault Injection):** `cmd/catalog` stopped &rarr; Gin times out at 2.0s &rarr; returns bounded HTTP 503.
- **T15 (F06 Service Recovery):** `cmd/catalog` restarted &rarr; gRPC recovers immediately.

---

## 5. Defect Analysis & AI Record

### Defect 1: TUI UI Thread Deadlock
- **Symptom:** Terminal UI froze on tab switch or incoming TCP/UDP event.
- **Root Cause:** In `tview`, calling `QueueUpdateDraw()` from within an active draw loop caused a self-deadlock on `<-ch`.
- **Fix:** Dispatched UI redraws into non-blocking goroutines (`go func() { s.App.QueueUpdateDraw(...) }()`).

### Defect 2: Protobuf Nil Message Info Panic
- **Symptom:** `UpdateChapters` RPC triggered `panic: invalid nil message info` in `cmd/catalog`.
- **Root Cause:** Hand-crafted message structs omitted pointer links to `file_proto_manga_proto_msgTypes[7]` and `[8]`.
- **Fix:** Properly mapped `ms.StoreMessageInfo(mi)` in `Reset()` and added descriptor methods.

### Defect 3: WebSocket Concurrent Map Write
- **Symptom:** Backend crashed with `fatal error: concurrent map writes` during client disconnection in chat broadcast.
- **Root Cause:** Unsafe `delete(h.clients, client)` was called while holding read lock `h.mu.RLock()`.
- **Fix:** Gathered dropped clients into a temporary slice under `RLock()`, then safely removed them under write `Lock()`.

### AI Collaboration Record
1. **Architecture Decoupling:** AI assisted in structuring Gin routes and separating monolithic files into focused packages.
2. **Review & Correction:** AI suggested synchronous `QueueUpdateDraw` calls; we tested, detected the deadlock, rejected the advice, and implemented decoupled background dispatches.
3. **Concurrency Hardening:** Used AI to review mutex lock patterns in the WebSocket Hub, identifying and eliminating the concurrent map mutation race condition.

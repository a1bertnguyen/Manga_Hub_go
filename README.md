# MangaHub — Distributed Manga Tracking & Real-Time Sync Platform

> **Course:** IT096IIU Net-Centric Programming — International University, VNU-HCM  
> **Target Specification:** Version 3.0 (Student Project Specification IT096IIU)  
> **Grade Target:** Level A / A+

---

## 1. System Overview

**MangaHub** is a production-grade, distributed net-centric application designed to manage manga catalogs, user libraries, and synchronized reading progress across multiple client sessions in real time. 

The architecture strictly follows the **3-Tier Distributed Architecture** mandated by Version 3.0 of the course specification, integrating **5 core network protocols** across **3 independent standalone processes**:

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

---

## 2. Architecture & 3 Standalone Processes

| Process | Source Entry | Protocols Provided / Consumed | Responsibility |
|---|---|---|---|
| **Manga Catalog Service** | `cmd/catalog/main.go` | **gRPC Server** (`:9092`) | Independent microservice hosting catalog search, manga details, chapter updates, and progress storage with SQLite `catalog.db` (seeded with 54 manga items across Shounen, Seinen, Shoujo, Josei, etc.). |
| **Unified Backend Gateway** | `cmd/backend/main.go` | **HTTP REST** (`:8080`), **TCP** (`:9090`), **UDP** (`:9091`), **WebSocket** (`:8080/ws`), **gRPC Client** (`:9092`) | Central coordinator handling JWT authentication, library CRUD, chapter overflow validation (HTTP 400), gRPC circuit-breaking / deadline fallback (HTTP 503), TCP progress sync with strict per-user isolation, UDP chapter release broadcasts, and multi-room WebSocket discussions. |
| **Interactive Terminal UI** | `cmd/tui/main.go` | **HTTP**, **TCP Client**, **UDP Listener**, **WebSocket Client** | Rich terminal application using `rivo/tview` & `gdamore/tcell/v2`. Features 6 screens (F01–F06), in-app notifications, live reading progress sync across devices, and live discussion rooms. |

---

## 3. Network Protocols Matrix

| Protocol | Port / Endpoint | Direction | Message Framing & Wire Format | Core Responsibility & Isolation Guarantees |
|---|---|---|---|---|
| **HTTP REST** | `:8080` (`/api/*`) | Client $\rightarrow$ Backend | JSON over HTTP/1.1 with Bearer JWT Auth | Registration, Login, Library management, Chapter progress update, Admin release trigger. |
| **gRPC** | `:9092` | Backend $\rightarrow$ Catalog | Binary Protocol Buffers (`proto/manga.proto`) | Microservice RPCs (`GetManga`, `SearchManga`, `UpdateChapters`, `UpdateProgress`). Uses 2s context deadline; maps `Unavailable` / `DeadlineExceeded` to HTTP 503. |
| **TCP** | `:9090` | Client $\leftrightarrow$ Backend | Newline-delimited JSON (`\n`) | Low-latency, reliable reading progress synchronization. **Strict User Isolation:** Updates from User A are broadcast *only* to connected devices authenticated as User A. |
| **UDP** | `:9091` | Client $\leftrightarrow$ Backend | UDP Datagrams (`SUBSCRIBE`, `UNSUBSCRIBE`, JSON alerts) | Lightweight broadcast for new chapter releases (`new_chapter` payload). Non-blocking and heartbeat-managed. |
| **WebSocket** | `:8080/ws` | Client $\leftrightarrow$ Backend | Full-duplex JSON (`chat`, `system`) | Community manga discussions with **Strict Room Isolation**: messages in room `shounen` never leak to room `seinen`. |

---

## 4. Key Functional Requirements (F01 – F06)

- **[F01] User Registration & Authentication:** Secure password hashing via `bcrypt` (cost 14) and HMAC-SHA256 JWT tokens. Duplicate usernames rejected with `409 Conflict`.
- **[F02] Manga Catalog Search:** Full catalog seeded with 54 manga items across Shounen, Seinen, Shoujo, Josei, etc. Query by keyword and filter by genre over gRPC.
- **[F03] Personal Reading Library:** Add manga to personal tracking library with reading statuses (`reading`, `completed`, `on-hold`, `dropped`).
- **[F04] Reading Progress & Concurrency Control:**
  - Chapter updates validate that `current_chapter <= total_chapters` (returns `400 Bad Request` if exceeded).
  - Optimistic concurrency control via `version` column in SQLite (`version = version + 1`).
  - Broadcasts progress over TCP exclusively to other devices belonging to the same user.
- **[F05] Live Chapter Announcements:** Admin chapter release endpoint triggers gRPC catalog update and broadcasts instant UDP alerts to all active subscribers.
- **[F06] Multi-room WebSocket Community Chat:** Real-time chat with room isolation, active participant tracking, and persistent recent history (last 50 messages).

---

## 5. Prerequisites & Quick Start Guide

### Prerequisites
- **Go 1.21** or higher installed ([golang.org](https://go.dev/))
- Git

### 1. Clone & Setup
```bash
git clone https://github.com/a1bertnguyen/Manga_Hub_go.git
cd Manga_Hub_go/mangahub-main/MangaHub-main
go mod tidy
```

### 2. Start the 3 Standalone Processes
Open 3 separate terminal windows:

#### Terminal 1 — Start Catalog Microservice
```bash
go run cmd/catalog/main.go
# Listens on gRPC :9092, initializes data/catalog.db with 54 seeded manga
```

#### Terminal 2 — Start Backend Gateway
```bash
go run cmd/backend/main.go
# Listens on HTTP/WS :8080, TCP :9090, UDP :9091, connects to gRPC :9092
```

#### Terminal 3 — Launch Interactive TUI Client
```bash
go run cmd/tui/main.go
```

---

## 6. TUI Client Keyboard Navigation

| Key | Screen / Action | Description |
|---|---|---|
| **F1** | Catalog Browser | Browse 54 seeded manga, filter by genre, add titles to library. |
| **F2** | My Library | View reading list, update current chapter, mark status. |
| **F3** | Discussion | Real-time chat rooms (`general`, `shounen`, `seinen`, `spoilers`). |
| **F4** | Notifications | View UDP chapter release announcements received in real time. |
| **F5** | Help & Status | System diagnostics, active protocol status, connection indicators. |
| **F10** | Login / Register | Modal dialog to authenticate or switch accounts. |
| **Ctrl+C** | Quit | Cleanly unregister workers and exit application. |

---

## 7. Systematic Automated Verification (Grade A Suite)

The codebase includes comprehensive automated tests covering all 5 protocols, boundary conditions, concurrency, and network failures:

```bash
# Run all tests in the repository
go test -v ./...
```

### Test Suite Breakdown:

| Test Package | Test File | Scenarios Verified |
|---|---|---|
| `internal/auth` | `auth_test.go` | Bcrypt hashing, password validation, JWT signing, expiration claims, token tampering rejection, unique ID generation. |
| `internal/database` | `persistence_test.go` | Schema initialization, 54 seed items loading, data persistence across restart (stop and re-open SQLite), optimistic concurrency locking (`version`). |
| `internal/grpc` | `service_test.go` | `GetManga` (200 / 404 NotFound), `SearchManga` (query & genre filtering), `UpdateChapters` (valid / negative chapters / not found), `UpdateProgress`. |
| `internal/tcp` | `hub_test.go` | **User Isolation Guarantee** (devices of User A receive updates; User B receives nothing), newline framing (`\n`), client disconnect cleanup. |
| `internal/udp` | `hub_test.go` | `SUBSCRIBE` / `UNSUBSCRIBE` lifecycle, chapter release datagram broadcasting, packet formatting, graceful socket termination. |
| `internal/websocket` | `hub_test.go` | **Room Isolation Guarantee** (messages in Room X never leak to Room Y), 50-message history buffer, client leave notifications. |
| `cmd/backend` | `backend_test.go` | **Full Integration Test**: Auth flow (200, 409 conflict, 401), **gRPC Failure / Service Unavailable (maps to 503)**, **Chapter overflow validation (chapter > total returns 400)**, Library persistence and version increment. |

---

## 8. Failure & Edge Case Scenarios

1. **Catalog Microservice Failure (HTTP 503):**
   - If `cmd/catalog` crashes or encounters latency > 2s, the Backend gRPC client deadline triggers and automatically converts the error to `HTTP 503 Service Unavailable`.
2. **Invalid Chapter Progress (HTTP 400):**
   - If a client attempts to update reading progress past the series' total chapter count, the backend cross-references the catalog via gRPC and rejects the request with `HTTP 400 Bad Request`.
3. **Data Persistence Across Restarts:**
   - Both user libraries and reading progress are stored in SQLite. Shutting down the backend and restarting preserves all reading history and incremented version counters.
4. **Network Drop / Disconnect Handling:**
   - TCP connections use heartbeat pings (`PING`/`PONG`) and clean EOF unregistration to prevent hanging goroutines or socket leaks.
   - UDP subscribers not heard from within 60 seconds are automatically garbage-collected.

---

## 9. Project Structure

```
MangaHub-main/
├── cmd/
│   ├── catalog/              # Standalone Catalog Microservice (:9092)
│   ├── backend/              # Unified Backend Gateway (:8080, :9090, :9091)
│   │   ├── main.go
│   │   └── backend_test.go   # Integration tests (503 handling, 400 validation)
│   └── tui/                  # Interactive Terminal UI Application
│       └── main.go
├── internal/
│   ├── auth/                 # Bcrypt hashing & JWT token management
│   ├── config/               # Environment & configuration loader
│   ├── database/             # SQLite connection & schema migrations
│   ├── grpc/                 # Manga catalog gRPC service implementation
│   ├── tcp/                  # TCP sync hub with per-user isolation
│   ├── udp/                  # UDP chapter notification hub & listener
│   ├── websocket/            # Multi-room discussion WebSocket hub
│   └── tui/                  # Modular tview client (views, API, TCP, UDP, WS)
├── pkg/models/               # Data structures & request/response models
├── proto/                    # Protocol Buffers definition & Go stubs
├── data/
│   └── manga_seed.json       # 54 curated manga items
├── go.mod
├── go.sum
└── README.md
```

---

## 10. Contributors & License

- **Course:** IT096IIU Net-Centric Programming
- **Institution:** International University — Vietnam National University, HCMC
- **Academic Year:** 2025–2026


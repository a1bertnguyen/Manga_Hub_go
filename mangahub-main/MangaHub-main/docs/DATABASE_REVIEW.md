# Database connections and persistence review

Reviewed on 23 November 2026. Canonical programs: `cmd/catalog`, `cmd/backend`, `cmd/tui`.

## Findings and changes

Discussion previously kept only the last 50 messages in the WebSocket Hub's RAM. No chat table or database write existed. Messages could be delivered live but did not survive restart.

The backend now creates `chat_messages` and its room/sequence index through an additive migration. A message stores an ID, verified user ID, username snapshot, room, text and UTC creation timestamp. SQLite commit must succeed before the Hub broadcasts the message or the sender receives its successful echo. A failed write reports an error and retains the TUI draft. The insert also verifies that the author exists in `users`.

On joining a room, the server sends its latest 50 stored messages, oldest first. This limit bounds replay; older rows remain in SQLite. Live/replay overlap is deduplicated by ID. Only chat messages are persisted; transient room membership and system leave events are not durable records.

Database initialization now closes a failed candidate connection, checks schema migration errors, and configures a three-second SQLite busy timeout. Old progress tables receive the version column only when it is absent. Registration checks now handle database read errors explicitly instead of continuing after ignored SQL errors.

## Data ownership and connection paths

| Data | Write/read path | SQLite owner |
|---|---|---|
| Accounts and password hashes | TUI -> protected/public auth HTTP handlers -> users | Backend: `data/mangahub.db` |
| Personal library, chapter/status/version | TUI -> authenticated HTTP -> user_progress; titles/limits validated via catalog gRPC | Backend: `data/mangahub.db` |
| Discussion messages | Authenticated WebSocket -> verified author/room -> committed chat_messages -> Hub broadcast; room replay on join | Backend: `data/mangahub.db` |
| Authoritative manga details/chapter count | Gin -> SearchManga/GetManga/UpdateChapters -> separate catalog process | Catalog: `data/catalog.db` |

The backend's older `manga` table is a legacy local copy, not the authoritative catalog. It currently has 34 rows versus 54 in the catalog service. Current discovery/details/release paths use gRPC. Do not inspect or edit the backend copy expecting an admin release to update it.

The old `cmd/api-server`, `cmd/grpc-server` and `cmd/websocket-server` are standalone demonstrations with different assumptions; they are not the assessed unified application. In particular, the old WebSocket program uses an in-memory Hub. Run the canonical three programs above.

## Actual local database audit

Both configured files were opened read-only for inspection. Neither password hashes nor chat/user contents were included in the retained JSON.

| Observation | Local result |
|---|---|
| Backend/catalog `PRAGMA quick_check` | Both `ok` |
| Backend accounts | 12; all password rows have a bcrypt prefix |
| Backend progress rows | 6 active, 2 archived |
| Authoritative catalog rows | 54 |
| Progress outside known chapter bounds / unknown statuses | 0 / 0 |
| Active progress rows with a missing user or catalog reference | 0 |
| SQLite `foreign_key_check` findings | 0 |
| Chat table | 1 persisted message at the time of the audit |

The two legacy progress rows had missing relationships. A transaction copied both rows and the reason for each move to `user_progress_archive` before removing them from the active library. The six active entries passed the relationship check afterward. The original rows also remain in the local SQLite backup at `data/backups/before-progress-repair-20261123-212934.db`. Catalog existence is validated through gRPC because its data is in a separate database. Chat writes verify the author in backend SQLite.

Before upgrading the real backend file, a consistent SQLite backup was created at `data/backups/before-chat-20261123-200444.db` (ignored by Git). The updated backend then ran the migration on the configured local file. Full before/after comparisons confirmed that existing `users`, legacy `manga` and `user_progress` rows were unchanged; the new chat table was added and integrity remained `ok`.

Re-run the audit from the repository root:

```powershell
python scripts/audit_databases.py --output mangahub-main/MangaHub-main/docs/verification/database-local-audit.json
```

Exit 0 means clean; exit 2 means findings were reported. The script is read-only. Paths come from process variables, then the local file; use `--backend` and `--catalog` for explicit file overrides. [Actual local audit JSON](verification/database-local-audit.json).

## Connected verification

```powershell
python scripts/verify_postman.py
python scripts/verify_connected.py --with-tui
```

The collection passes 64 HTTP requests and 128 assertions. The database roundtrip includes 15 checks across SQLite integrity, bcrypt storage, same-room delivery, chat row readback, room isolation, chapter updates, process restart, account login, progress persistence and history replay in the TUI.

Store/unit tests additionally cover the last-50 replay limit without deleting older rows, old-schema migration without data loss, unknown author rejection, SQLite read-only failure without a successful echo, retained TUI draft, and controlled HTTP 500 errors when the database is closed. Full Go race tests and vet pass.

- [Connected database/restart results and TUI log](verification/postman-feature-acceptance.json)
- [Full database-change race suite](verification/database-race-tests.txt)

## Inspect chat rows

Refresh the backend data source in GoLand after migration, then run this against `mangahub.db`:

```sql
SELECT sequence, id, username, room, text, created_at
FROM chat_messages
ORDER BY sequence DESC
LIMIT 50;
```

Restart backend and TUI to use durable chat and the history frame. Catalog can remain running. Messages sent before persistence was implemented were only in RAM and are not retroactively present in the new table.

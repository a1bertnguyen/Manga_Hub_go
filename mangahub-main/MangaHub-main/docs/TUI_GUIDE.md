# MangaHub TUI guide

The redesigned client keeps the Go/tview interface and the existing backend protocols. Use Go 1.25.1 or later. Run these commands from `mangahub-main/MangaHub-main`, each in a separate terminal:

```powershell
go run ./cmd/catalog
```

```powershell
go run ./cmd/backend
```

```powershell
go run ./cmd/tui
```

Open Account with F10 to register or log in. Catalog access requires authentication on the existing backend. An earlier saved session is restored automatically. If it has expired, log out and sign in again.

Catalog, backend and TUI automatically load `.env.local` from the working directory. Keep the file in the Go module directory used above. Existing PowerShell environment variables take priority; open a fresh terminal if an old override should no longer apply. Restart programs after editing the file. The local file is ignored by Git.

## Keyboard input regression fix

The centered Account/Profile and progress-dialog container previously forwarded focus and mouse events but inherited `Box`'s empty keyboard/paste handlers. As a result, terminal key events passed through the root layout and stopped at that container before reaching the visible input or button. The older test helper called the focused field directly, bypassing this failing path.

The container now forwards keyboard and paste events to its child, terminal paste is enabled, and the root is mounted before session restoration chooses the initial focus. The helper now dispatches through Pages. Four additional tests inject events through the actual tcell/tview event loop and cover startup typing, mouse navigation followed by login submission, progress editing/paste, search/chat text, and Tab/Shift+Tab navigation. See [the retained TUI regression output](verification/tui-input-regression.txt).

If an older TUI process is still running, exit it with Ctrl+C and rerun `go run ./cmd/tui` from the module directory. The backend and catalog can remain running for this client-only fix. PowerShell/Windows Terminal is supported.

## Navigation

| Key | Action |
|---|---|
| F1 / F2 / F3 / F4 / F5 / F10 | Catalog / Library / Discussion / Alerts / Connections / Account |
| 1 / 2 / 3 / 4 / 5 / 0 | Page shortcuts outside text inputs, dropdowns and account forms |
| Tab / Shift+Tab | Next / previous control within the current page |
| Alt+Right / Alt+Left | Next / previous page |
| Mouse click on the navigation | Open that page |
| Ctrl+C / Alt+Q | Quit and close client workers |

## Catalog and library

Press `/` to focus search, type a title or author, then Enter. Press `g` to choose a genre. `r` refreshes with the current filters. Use the arrow keys to select a title; its details appear beside the list, or below it in a narrow terminal. Tab moves into details so the synopsis can be scrolled.

Enter or `a` opens the add-to-library dialog. In Library, Enter edits chapter/status; `+` and `-` change the chapter directly. Escape cancels the progress dialog. The visible library retains its selected row during refresh. Progress totals use the current catalog cache; an unavailable total displays as unknown.

## Discussion and chapter alerts

Discussion opens at the message field. Tab moves to rooms, then conversation history, then back to the message. Select a room with the arrow keys and Enter. Enter sends a non-empty message asynchronously. If a send fails, the message remains available for retry.

Discussion delivery was strengthened on 23 November 2026. The earlier server limit of 512 bytes could close the socket on a Vietnamese paragraph while the client had already cleared the input. Both sides now use a 16 KiB encoded-frame limit; an oversized message is rejected locally without losing the text or disconnecting. Username, text and timestamps are escaped before tview rendering, so strings such as `[red]` are displayed literally. Queued frames from a connection that has been replaced are ignored.

Every TUI message carries a random ID echoed by the server through its room Hub. The client waits up to five seconds for that matching echo before clearing the input. A socket close or missing confirmation shows an error and retains the text. The echo confirms the server returned that message to the sending TUI; delivery to peers is checked separately in the connected tests. There is no automatic retry.

The updated canonical backend commits Discussion messages to `chat_messages` in backend SQLite before broadcasting/confirming them. Joining a room replays its latest 50 stored messages in chronological order, including after a backend restart. The TUI clears the previous room view before replay and deduplicates replay/live overlap by message ID. A failed SQLite write produces an error and retains the draft. See [the database review](DATABASE_REVIEW.md) for the actual file paths, local data findings and restart evidence. The older standalone WebSocket demo still uses memory only.

An HTTP 200 in Gin's log for `/ws` is not a per-message delivery receipt. The route hijacks the HTTP connection for WebSocket; actual chat frames travel over the socket afterward. Check the Conversation pane and the send feedback. Restart both backend and TUI after this protocol update; catalog can stay running.

Run `go test -race ./internal/tui -run TestDiscussion -count=1 -v` from the module directory. These tests use the real WebSocket Hub and tcell event loop, including inspecting rendered screen cells on sender and peer at 120x38 and 80x24, literal formatting characters, Vietnamese messages, delayed/missing echo, oversized input and disconnection. See [the focused delivery tests](verification/discussion-delivery-tests.txt) and [the full race test output](verification/discussion-race-tests.txt).

In Alerts, `s` subscribes, `u` unsubscribes, `c` clears the feed, and `t` triggers the protected demo release endpoint. Set the same private `ADMIN_API_KEY` in the backend and teacher/demo TUI shell; a logged-in account is also required. Empty configuration disables release. A demo release changes the catalog's chapter count; use a disposable demo database when demonstrating this action. UDP delivery is best effort.

Connections shows the configured client addresses, backend/catalog health, and client connection flags. `r` refreshes it. The implemented identity checks and verification results are documented in [the grade readiness review](GRADE_A_REVIEW.md).

## Verification and previews

```powershell
go test ./internal/tui -count=1
go test -race ./internal/tui -count=1
```

The following previews were captured from the actual tcell renderer using fixture manga data. They illustrate layout, not a live networking demo:

![Catalog at 120 columns](images/tui-catalog-preview.png)

![Account at 80 columns](images/tui-account-preview.png)

For repeatable renderer snapshots:

```powershell
$env:TUI_CAPTURE_DIR = Join-Path $env:TEMP 'mangahub-tui-captures'
go test ./internal/tui -run TestRedesignScreens -count=1
```

This writes per-cell JSON snapshots for all six screens at 120x38 and 80x30. It does not modify the user's database or saved session.

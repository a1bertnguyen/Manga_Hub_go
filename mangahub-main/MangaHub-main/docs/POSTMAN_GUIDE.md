# MangaHub Postman guide

Import these two files from this directory:

- `MangaHub_API.postman_collection.json`
- `MangaHub_Local.postman_environment.json`

Select **MangaHub Local (demo)** as the active environment. Start `go run ./cmd/catalog` and `go run ./cmd/backend` in separate terminals from the Go module directory. Keep the default `baseUrl=http://localhost:8080`, or change it to the actual backend address.

## Normal collection run

Run the collection in its saved order. It contains 57 requests in nine folders. Folders 01-05 and 07 form the ordinary workflow; the three manual folders are skipped by default. Each registration generates a new demo username, email and password, and successful authentication stores the JWT and identity automatically. Catalog requests select real manga IDs, so seed slugs are not hardcoded.

| Folder | What it checks |
|---|---|
| 01 Health | HTTP response and connected gRPC catalog; this is not TCP/UDP delivery evidence |
| 02 F01 Accounts | Registration, duplicate rejection, password validation, login, wrong password, unauthorized access |
| 03 F02 F06 Catalog | Catalog list, title search, genre filter, no results, all six detail fields, unknown ID |
| 04 F03 Library and progress | Plan-to-read, reading and completed; chapter zero; persistence/version checks; negative/overflow/above-total chapter rejection |
| 05 F01 User isolation and admin guards | Separate Bob account cannot read/edit Alice's entry; admin authentication and key checks |
| 06 F04 Admin demo | Opt-in real release, updated count, invalid release inputs and unknown ID |
| 07 Legacy route aliases | Existing login/catalog/library/progress aliases |
| 08 F06 Catalog offline | Opt-in controlled 503 responses, including unchanged stored progress |
| 09 Restart and recovery | Opt-in account login and unchanged library after real service restart |

Runtime variables include `token`, `userId`, `testUsername`, `password`, `mangaId`, `totalChapters`, `lastVersion`, `expectedChapter`, `bobToken` and `bobUserId`. They are set in the active environment and collection. Use the generated credentials only with a disposable demo database; repeated normal runs create new demo users. Do not export/share an environment containing live tokens or private keys.

Negative requests intentionally expect 400, 401, 403 or 404. A green test on such a request means rejection worked. The collection does not claim that an HTTP success response proves a TCP/UDP event was displayed in a TUI.

## Authorized chapter release and UDP demonstration

Set `ADMIN_API_KEY` before launching the backend. Set the Postman environment's `adminKey` to the same private value. Run the normal workflow first, then set `runAdminDemo=true` and run **only folder 06**. This changes the authoritative chapter count through gRPC and broadcasts a UDP event.

Keep two logged-in TUI clients subscribed (`s` on Alerts). Unsubscribe a third client (`u`). The two subscribed TUIs should render the new-chapter alert; the unsubscribed client should not. An HTTP 200 from the release request only confirms the release endpoint's outcome, not datagram delivery.

Restore `runAdminDemo=false` after the demonstration.

## Catalog outage and restart demonstration

1. Complete the normal workflow while both services are running.
2. Stop **only the catalog process** with Ctrl+C.
3. Set `runFaultChecks=true` and select **only folder 08** in Runner. Search/details/add/progress must return controlled 503 responses. The final library request must retain the chapter and version recorded earlier.
4. Restart catalog. Restart backend with the same database paths and `JWT_SECRET` if demonstrating persistence.
5. Set `runRecoveryChecks=true`, then run **only folder 09**. The previously created user must still log in and the saved library must match.
6. Return both flags to `false` afterward.

Do not enable the outage flag and run the entire collection while catalog is down; the earlier normal search workflow requires a working catalog.

## F05 WebSocket in Postman

The HTTP collection format does not represent a raw WebSocket conversation. After the normal workflow has saved both account tokens, create **New > WebSocket** separately:

- URL: `{{wsUrl}}?room=general`
- Header before Connect: `Authorization: Bearer {{token}}`
- Message format: JSON

```json
{"type":"chat","text":"Hello from Alice"}
```

Open a second WebSocket tab in `general` with `Authorization: Bearer {{bobToken}}`. Send messages in both directions. Open another tab with `?room=spoilers`; it must not receive messages from `general`. Disconnect Bob's general tab and confirm Alice's room still works.

The server sets the username from JWT claims and room from the accepted connection. A client-supplied `username` or `room` inside a message cannot override those values. Connecting without a valid Bearer token must return 401. For the assessed feature, repeat the exchange in the TUI as well.

Postman's native WebSocket setup and header instructions: [create a WebSocket request](https://learning.postman.com/v11/docs/use/send-requests/protocols/websocket/create-a-websocket-request), [configure headers before connecting](https://learning.postman.com/latest-v-12/docs/use/send-requests/protocols/websocket/add-details-websocket).

## F06 Direct gRPC inspection

Create a separate gRPC request to `{{grpcAddress}}` and import the project's `proto/manga.proto` as its service definition. The catalog program does not register server reflection, so use the committed proto file.

- `manga.MangaService/SearchManga`: `{"query":"One","genre":""}`
- `manga.MangaService/GetManga`: `{"id":"{{mangaId}}"}`
- Unknown ID: `{"id":"__postman_missing_id__"}` must return gRPC NotFound.

The internal gRPC listener has no HTTP Bearer middleware. It is for the trusted local service boundary, not a public client API. Use the protected HTTP release request for the chapter-release demonstration. The legacy `UpdateProgress` RPC is not the backend's personal-library write path.

## TCP and UDP

Raw TCP progress and UDP datagrams are verified through actual TUI clients and the project's socket verifier. Do not substitute an HTTP request named TCP/UDP and count its response as delivery proof. See [the live demo checklist](DEMO_CHECKLIST.md) and [TUI guide](TUI_GUIDE.md).

TCP authentication returns a server-issued `session_id`. A TUI progress request includes it in the `X-Session-ID` header so the backend can exclude that originating TCP connection. An unknown, disconnected or different-account identifier is rejected with HTTP 400 before any write. The ordinary HTTP-only collection has no TCP origin and omits the header; all same-account TCP clients are its peers. `verify_postman.py` additionally verifies source exclusion on real TCP sockets for both progress routes and after reconnect.

## Automated verification of these exact files

From the repository root, with Go, Python and Node/npm installed:

```powershell
python scripts/verify_postman.py
```

The command uses Newman 6.2.3 to execute the importable collection against isolated real backend/catalog processes, including controlled outage and restart phases. It also exercises actual TUI form/buttons, search/details, reading statuses, peer TCP redraw, alerts and chat under the race detector. Temporary databases and credentials are removed when the run finishes. Only a sanitized summary is retained in `docs/verification/postman-feature-acceptance.json`; raw Newman exports containing credentials stay temporary.

To run only the ordinary collection against your own local services:

```powershell
npx --yes newman@6.2.3 run mangahub-main/MangaHub-main/docs/MangaHub_API.postman_collection.json -e mangahub-main/MangaHub-main/docs/MangaHub_Local.postman_environment.json
```

Regenerate the two JSON artifacts with `python scripts/generate_postman.py` after editing their generator. Keep any private Postman environment separate from these secret-free templates.

References: [Postman script-controlled skipping](https://learning.postman.com/v11/docs/tests-and-scripts/write-scripts/postman-sandbox-reference/pm-execution), [Newman options](https://learning.postman.com/docs/collections/using-newman-cli/newman-options), [environment variables](https://learning.postman.com/docs/use/send-requests/variables/environment-variables).

---
feature: ws-recording
status: delivered
updated: 2026-08-30
branch: feature/ws-recording
commits: (pending)
---

# WebSocket Recording System

## Report

**What was built** — A WebSocket recording system that allows a spectator to capture all WebSocket traffic (send and receive) while performing game actions on jackbox.tv. The system consists of three parts: a page-context WebSocket interceptor (`ws-interceptor.js`) that monkey-patches `WebSocket.prototype` to capture all messages; an extension UI overlay with start/stop recording controls, action naming, and notes; and a Go server endpoint (`POST /v1/recording`) that persists recordings as JSON files in a `recordings/` directory.

**Verification** — `go build ./...` PASS, `go vet ./...` PASS, `manifest.json` valid JSON, `content.js` syntax OK, `ws-interceptor.js` syntax OK, `sw.js` syntax OK. Review found 3 critical issues (Russian comments in recording.go, binary handling on recv path, missing started_at/stopped_at validation) — all fixed and re-verified.

**Journey log**:
- Initial subagent dispatches timed out without producing output; implemented directly instead.
- Reviewer caught that the recv path used simplified `"[binary]"` fallback instead of reusing the existing `postCapture()` function that already handled Blob/ArrayBuffer correctly.
- Added `started_at`/`stopped_at` validation after reviewer noted zero values would produce degenerate filenames.

## [S1] Problem

The botnet currently supports a fixed set of Jackbox games (Trivia Death 2, Quiplash, Everyday, Poll Position). Adding support for new game modes requires manually reverse-engineering the WebSocket protocol by reading network traffic in DevTools — a tedious and error-prone process. There is no structured way to capture what messages a real spectator sends and receives during specific game actions (voting, answering, etc.), which makes it hard to write correct botnet handlers for new games.

## [S2] Design

### Architecture

Three components cooperate:

1. **Content script** (`extension/content.js`) — injects a page-context script that monkey-patches `WebSocket.prototype.send` and the `onmessage` setter to intercept all WebSocket traffic on `jackbox.tv`. Forwards captured messages to the content script via `window.postMessage`.

2. **Extension UI** — overlay panel with recording controls (start/stop), action name input, and optional note textarea. On stop, sends the recorded session to the Go server via the service worker.

3. **Go server** (`POST /v1/recording`) — receives recording payloads and persists them as JSON files under `recordings/` directory relative to the server's working directory.

### WebSocket Interception

The injected page-context script wraps `WebSocket.prototype`:

- **`send` wrapper**: before the original `send`, captures `event.data` as `{ dir: "send", ts: <ms>, data: <string> }` and posts it to the content script.
- **`onmessage` setter wrapper**: when the page sets `onmessage`, wraps the handler to also capture each message event as `{ dir: "recv", ts: <ms>, data: <string> }`.
- **`addEventListener` wrapper**: intercepts `message` event listeners added via `addEventListener` to capture received messages.

Messages are buffered in the content script only while recording is active. Binary messages (Blob/ArrayBuffer) are converted to string via `TextDecoder` or logged as `[binary N bytes]` if non-UTF8.

### Recording Lifecycle

1. User clicks "Start recording" in the overlay.
2. Extension sets `recording = true`, clears the buffer, records `started_at` timestamp.
3. All intercepted WebSocket messages are appended to the buffer.
4. User performs the game action in the browser.
5. User clicks "Stop recording".
6. A modal/dialog appears with:
   - **Action name** (required, free text) — e.g. "vote_for_answer", "submit_prompt"
   - **Note** (optional, free text) — e.g. "Variants: 0-Cat, 1-Dog. Chose 1."
7. On confirm, the extension sends the recording to the Go server.

### Recording Payload (JSON)

```json
{
  "action_name": "vote_for_answer",
  "note": "Variants: 0-Cat, 1-Dog, 2-Parrot. Chose 1.",
  "page_url": "https://jackbox.tv/...",
  "started_at": 1719000000000,
  "stopped_at": 1719000012000,
  "messages": [
    { "dir": "recv", "ts": 1719000001000, "data": "{\"opcode\":\"object\",...}" },
    { "dir": "send", "ts": 1719000005000, "data": "{\"seq\":1,...}" }
  ]
}
```

### Go Server Endpoint

`POST /v1/recording`

- Auth: `X-JF-Token` header (same as `/v1/event`).
- Body: JSON recording payload.
- Validation: `action_name` must be non-empty, `messages` must be a non-empty array, each message must have `dir` ("send" or "recv"), `ts` (number), and `data` (string).
- Storage: saves to `recordings/<timestamp>-<action_name>.json` (sanitized action name, filesystem-safe).
- Response: `{ "ok": true, "path": "recordings/..." }`.

### File Storage

Recordings are saved as individual JSON files in a `recordings/` directory created at server startup. Filename format: `<unix_ms>-<sanitized_action_name>.json`. The action name is sanitized by replacing non-alphanumeric characters with underscores and truncating to 64 characters.

### Extension UI

The overlay panel adds:
- A toggle button "Start recording" / "Stop recording" (red when recording).
- When recording is stopped, a small form slides down with:
  - Text input for action name (required).
  - Textarea for note (optional).
  - "Save" and "Cancel" buttons.
- Status indicator showing recording state and message count.

### Auth Flow

The extension already stores `port` and `token` in `chrome.storage.local`. The recording submission uses the same token and port as the existing event submission — no new auth configuration needed.

## [S3] Out of Scope

- CLI commands for viewing/browsing recordings (`jackfools recordings list/show`).
- Automatic generation of botnet handlers from recordings.
- Recording playback or replay.
- Support for games outside `jackbox.tv`.
- Binary WebSocket message parsing (logged as `[binary N bytes]`).
- Export/import of recordings.

## Tasks

- [x] T1: Inject page-context WebSocket interceptor script — acceptance: content script injects a script that wraps `WebSocket.prototype.send`, `onmessage`, and `addEventListener("message")`, posting captured messages to the content script via `window.postMessage` (covers: S2)
- [x] T2: Add recording buffer and controls to content script — acceptance: content script maintains a message buffer, responds to start/stop recording commands, and forwards buffered messages to the service worker (covers: S2)
- [x] T3: Build recording UI overlay — acceptance: overlay shows "Start/Stop recording" button, action name input, note textarea, save/cancel buttons, and recording status with message count (covers: S2)
- [x] T4: Add service worker recording submission — acceptance: service worker sends `POST /v1/recording` with the recording payload and auth token, handles success/error responses (covers: S2)
- [x] T5: Add `POST /v1/recording` endpoint to Go server — acceptance: endpoint validates payload, creates `recordings/` directory if needed, saves JSON file, returns `{ "ok": true, "path": "..." }` (covers: S2)
- [x] T6: Add recording validation to Go server — acceptance: server rejects empty `action_name`, empty `messages`, invalid `dir` values, and missing required fields with 400 status and descriptive error (covers: S2)

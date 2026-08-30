---
feature: trivia-death-2-audience-bot
status: delivered
updated: 2026-08-30
branch: feature/trivia-death-2-audience-bot
commits: 4381ea7..HEAD
---

# Trivia Death 2 — Audience Bot

## Report

**What was built** — A Chrome extension audience bot for Trivia Death 2 that automatically detects game phases via WebSocket message parsing, votes on trivia questions using a learned question bank, and teaches itself correct answers from game reveals. The bot handles regular questions (random vote when unknown, correct vote when known), death room votes (always random), and final round multi-select questions (skip when unknown, correct subset when known). A Ctrl+Shift+J shortcut toggles between auto and manual mode, with a live overlay showing mode status, current question, and bank size.

**Verification** — Parser tested against all 4 relevant recording files: all event types correctly classified (regular questions, death room votes, final round questions, correct answers singular/plural, voting closed, game credits, and all textDescription subtypes). Integration test verified teaching flow produces correct bank entries from both regular and final round recordings. Go server compiles clean with new question bank endpoints.

**Journey log** — (1) Two WebSocket protocol formats exist in the wild: `jackbox.tv` (test_vote.json, different protocol, excluded) and `jackbox.fun` (all TD2 recordings). Parser targets the `jackbox.fun` format exclusively. (2) Death room detection uses prompt keyword regex rather than temporal proximity to `KILLING_FLOOR_PLAYERS` event, which is simpler and covers all observed cases. (3) Final round votes require comma-separated indices (e.g. "0,2") rather than single index, with the count-group containing all subset keys.

## [S1] Problem

The JackFools extension can record WebSocket traffic from Trivia Death 2 games, but has no mechanism to automatically participate as an audience member. A spectator bot that could vote on trivia questions, learn correct answers from game reveals, and build a question bank over time would give the audience a significant advantage. Currently there is no structured way to detect game phases (regular questions, death room votes, final round, game end) or to act on them programmatically.

## [S2] Design

### Architecture

Four components:

1. **Message Parser** (`extension/td2-parser.js`) — classifies incoming WebSocket messages into game events: regular question, death room vote, final round question, correct answer reveal, game credits. Extracts structured data (prompt, choices, correct answer text, round type).

2. **Question Bank** (`client/questions.json`) — JSON file mapping question prompts to correct answer texts. One entry per unique prompt string (including `[i]` tags). Different translations of the same question produce different entries. Managed by the Go server via REST endpoints.

3. **Auto-Voter** (logic inside `extension/content.js`) — when a question arrives and auto-mode is active, looks up the prompt in the bank. If found, sends a vote through the WebSocket. If not found, votes randomly (regular/death room) or skips (final round). Togglable via keyboard shortcut `Ctrl+Shift+J`.

4. **Teaching Loop** (logic inside `extension/content.js`) — when a `TEXT_DESCRIPTION_CORRECT_ANSWER(S)` message arrives, extracts the correct answer text, pairs it with the last question's prompt, and sends it to the server for storage in the bank.

### Message Classification (from recordings)

The parser inspects incoming JSON messages and classifies them by `opcode` + `result.key` + nested fields:

| Event | Detection | Extracted Data |
|---|---|---|
| **Regular question** | `opcode: "object"`, `key: "audiencePlayer"`, `val.kind: "choices"`, no `val.roundType` | `prompt`, `choices[]`, `countGroupKey` |
| **Final round question** | same as above but `val.roundType === "FinalRound"` | `prompt`, `choices[]`, `countGroupKey` |
| **Death room announced** | category `TEXT_DESCRIPTION_KILLING_FLOOR_PLAYERS` | player names entering death room |
| **Death room vote** | `opcode: "object"`, `key: "audiencePlayer"`, `val.kind: "choices"`, preceded by death room announcement within last 5s, OR prompt matches `/умрёт|смерти|комнату смерти/i` | `prompt`, `choices[]` (2-N player names, count depends on lobby size) |
| **Death room result** | category `TEXT_DESCRIPTION_KILLING_FLOOR_PLAYER_KILLED` | player name who died |
| **Voting closed** | `opcode: "object"`, `key: "audiencePlayer"`, `val.kind: "waiting"` | — |
| **Correct answer (singular)** | `opcode: "object"`, `key: "textDescriptions"`, category `TEXT_DESCRIPTION_CORRECT_ANSWER` | `text` — "Верный ответ: X" |
| **Correct answers (plural)** | same, category `TEXT_DESCRIPTION_CORRECT_ANSWERS` | `text` — "Верные ответы: X и Y" |
| **Correct player(s)** | category `TEXT_DESCRIPTION_QUESTION_CORRECT_PLAYER` or `TEXT_DESCRIPTION_QUESTION_CORRECT_PLAYERS` | player name(s) who answered correctly |
| **Final round lead swap** | category `TEXT_DESCRIPTION_FINAL_ROUND_LEAD_SWAPPED_PLAYER` | player names |
| **Final round lead** | category `TEXT_DESCRIPTION_FINAL_ROUND_LEAD_PLAYER` | leading player name |
| **Final round devoured** | category `TEXT_DESCRIPTION_FINAL_ROUND_DEVOURED_PLAYER` | player name who died |
| **Final round audience devoured** | category `TEXT_DESCRIPTION_FINAL_ROUND_DEVOURED_AUDIENCE` | — |
| **Final round blocked** | category `TEXT_DESCRIPTION_FINAL_ROUND_BLOCKED_PLAYER` | player name |
| **Final round escaped** | category `TEXT_DESCRIPTION_FINAL_ROUND_ESCAPED_PLAYER` | player name who escaped |
| **Game credits** | `opcode: "artifact"` | `artifactId`, `categoryId` |
| **End game deaths** | category `TEXT_DESCRIPTION_END_GAME_CAUSE_OF_DEATH_PLAYER` | player name, money, cause |
| **End game survivor** | category `TEXT_DESCRIPTION_END_GAME_SURVIVOR_PLAYER` | player name, money |

### Vote Sending

The `ws-interceptor.js` must be extended to support outbound voting:

1. Track the most recent WebSocket instance on `WebSocket.prototype` construction.
2. Listen for `CustomEvent("__JF_VOTE__", { detail: { name, vote, times } })` dispatched on `window`.
3. On receipt, construct the JSON payload `{"seq": N, "opcode": "audience/count-group/increment", "params": {name, vote, times}}` and send it through the tracked WebSocket.
4. Maintain a `seq` counter starting from 1 (incremented per vote).

### Voting Strategy

**Auto-mode (default):**

| Round Type | Bank Hit | Bank Miss |
|---|---|---|
| Regular question | Vote for correct answer index | Vote random index (0-3) |
| Death room vote | N/A (no correct answer) | Vote random index (0 to choices.length-1) |
| Final round | Vote for correct answer indices (comma-separated) | Skip (don't vote) |

**Manual mode:** Extension does nothing; user votes via the jackbox.fun UI normally.

**Toggle:** `Ctrl+Shift+J` switches between auto and manual. A small indicator in the overlay shows current mode.

**Note on death room votes:** These are predictions, not trivia. The bot always votes randomly regardless of bank state. Death room outcomes are never stored in the question bank.

### Teaching Flow

1. A question arrives (regular or final). Parser extracts `prompt` and `choices[]`.
2. The bot stores the current question context: `{ prompt, choices, timestamp }`.
3. The bot votes per strategy.
4. When `TEXT_DESCRIPTION_CORRECT_ANSWER` or `TEXT_DESCRIPTION_CORRECT_ANSWERS` arrives:
   - Parse the answer text: "Верный ответ: X" → `["X"]`; "Верные ответы: X и Y" → `["X", "Y"]`.
   - For each answer text, find its index in the stored `choices[]` by matching `choice.text === answerText`.
   - Send `POST /v1/questions` to the server with `{ prompt, answers: [{ text, index }] }`.
5. Server stores/updates the entry in `questions.json`.

### Answer Text Parsing

The correct answer text from `textDescriptions` uses these formats:
- Singular: `"Верный ответ: <text>"` — extract `<text>` after ": ".
- Plural: `"Верные ответы: <text1> и <text2>"` — split by " и ", extract each.

The `[i]...[/i]` markup tags are stripped before matching against choices but preserved in the stored prompt for exact future lookups.

### Question Bank Format (`client/questions.json`)

```json
{
  "questions": [
    {
      "prompt": "В каком году появился первый велосипед?",
      "answers": [
        { "text": "1817", "index": 3 }
      ],
      "seen_count": 1,
      "last_seen": 1788086120991
    },
    {
      "prompt": "Какого из этих милых зверьков не присутствовало в [i]Kirby's Dream Land 2[/i]?",
      "answers": [
        { "text": "Кошка", "index": 1 }
      ],
      "seen_count": 1,
      "last_seen": 1788086177928
    }
  ]
}
```

For final round questions with multiple correct answers:
```json
{
  "prompt": "Основные и вспомогательные органы пищеварительной системы",
  "answers": [
    { "text": "Сигмовидная кишка", "index": 0 },
    { "text": "Аппендикс", "index": 2 }
  ],
  "seen_count": 1,
  "last_seen": 1788085033131
}
```

Note: `index` is the position in the choices array at the time of the question. Different game sessions may shuffle choices, so the bot must match by `text`, not by stored `index`. The `index` field is informational only.

### Server API Endpoints

**`GET /v1/questions`** — returns the full question bank.

**`POST /v1/questions`** — adds or updates a question entry.
- Body: `{ "prompt": "...", "answers": [{ "text": "...", "index": N }] }`
- If `prompt` already exists: merge answers (update `seen_count`, `last_seen`, add new answer texts not already present).
- Response: `{ "ok": true }`.

**`GET /v1/questions/lookup?prompt=...`** — looks up a question by exact prompt match.
- Response: `{ "found": true, "answers": [...] }` or `{ "found": false }`.

### Extension UI Changes

Add to the existing overlay:
- **Mode indicator**: "AUTO" (green) or "MANUAL" (yellow) label.
- **Mode toggle button**: "Switch to Manual" / "Switch to Auto".
- **Last question display**: shows the current question prompt (truncated) and whether the answer is known.
- **Question count**: "Bank: N questions" label.
- **Keyboard shortcut**: `Ctrl+Shift+J` toggles mode (handled via `keydown` listener on `document`).

### File Changes

| File | Change |
|---|---|
| `extension/td2-parser.js` | New file. Message classifier + data extractor. |
| `extension/content.js` | Add auto-vote logic, teaching loop, UI additions, keyboard shortcut. |
| `extension/ws-interceptor.js` | Add WebSocket instance tracking + `__JF_VOTE__` event listener. |
| `extension/sw.js` | Add message handlers for `JF_QUESTION_LOOKUP`, `JF_QUESTION_STORE`, `JF_QUESTIONS_LIST`. |
| `client/questions.json` | New file. Question bank (created by server). |
| Go server | Add `GET /v1/questions`, `POST /v1/questions`, `GET /v1/questions/lookup` endpoints. |

## [S3] Out of Scope

- Multi-language question deduplication (each translation is a separate entry).
- Question bank export/import.
- Analytics on question difficulty or audience accuracy.
- Support for games other than Trivia Death 2.
- Automatic question scraping from external sources.
- Persisting auto/manual mode preference across sessions.

## Tasks

- [x] T1: Create `extension/td2-parser.js` with message classifier — acceptance: parses all 5 recording files and produces correct event types for each message (covers: S2 Message Classification)
- [x] T2: Extend `extension/ws-interceptor.js` with WebSocket instance tracking and `__JF_VOTE__` event listener — acceptance: dispatching a `CustomEvent("__JF_VOTE__")` on `window` sends the correct JSON through the active WebSocket (covers: S2 Vote Sending)
- [x] T3: Add `GET /v1/questions`, `POST /v1/questions`, `GET /v1/questions/lookup` endpoints to Go server — acceptance: can store, retrieve, and lookup questions via HTTP; duplicate prompts merge answers and increment `seen_count` (covers: S2 Server API Endpoints, S2 Question Bank Format)
- [x] T4: Add auto-vote logic to `extension/content.js` — acceptance: when auto-mode is active and a known question arrives, the extension sends a vote within 500ms; when unknown, votes randomly for regular questions or skips for final round (covers: S2 Voting Strategy)
- [x] T5: Add teaching loop to `extension/content.js` — acceptance: when a correct answer reveal message arrives, the extension extracts the answer text, pairs it with the stored question context, and sends `POST /v1/questions` to the server (covers: S2 Teaching Flow)
- [x] T6: Add UI elements and keyboard shortcut to `extension/content.js` — acceptance: overlay shows mode indicator, toggle button, last question, bank count; `Ctrl+Shift+J` toggles auto/manual mode (covers: S2 Extension UI Changes)
- [x] T7: Integration test with recording playback — acceptance: replaying `vote_for_usual_question_teaching.json` through the parser produces a question bank entry with the correct answer; replaying `final_for_zriteli.json` produces final round entries with multiple answers (covers: S2)

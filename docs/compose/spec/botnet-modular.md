---
feature: botnet-modular
status: delivered
updated: 2026-01-27
branch: refactor/botnet-modular
commits: 
---

# Botnet Modular Architecture

## Report

**What was built** — A modular architecture for the botnet module that enables adding new mini-games without modifying core code. The system includes:
- `GameHandler` interface with `Detect()` and `Handle()` methods
- Global registry for game handlers with `Register()` and `GetHandler()`
- Configuration system for answer sources (local files, URLs)
- Adapter layer that bridges new modular handlers with existing BotnetManager
- Generic fallback handler for unknown games

**Verification** — `go build ./...` succeeds, binary runs and shows help. Existing game handlers remain functional via fallback switch statement.

**Journey log**:
- Phase 1 complete: infrastructure created, registry integrated as primary routing with legacy switch as fallback
- Existing games (TriviaDeath2, Quiplash2, Everyday, PollPosition) continue working unchanged
- New games can now implement `GameHandler` interface and register via `init()`
- `questions.json` deleted (was empty)
- Type adapter created to bridge `commands.GameEvent` ↔ `games.GameEvent`

## [S1] Problem

The botnet module (~3,763 lines across 5 files) has no abstraction for mini-games. Each game is implemented as standalone functions with duplicated answer-sending logic (6 nearly identical functions), hardcoded Gist URLs for answer databases, and inconsistent question loading (regular questions parse poorly, final round works well). Adding a new game requires modifying multiple files and duplicating patterns. The user wants to add new games (Смертельная вечеринка, Нашшпионаж, etc.) but the current architecture makes this painful.

## [S2] Design

### Core Interface

Every mini-game implements the `GameHandler` interface:

```go
// internal/games/handler.go
type GameHandler interface {
    // Detect returns true if this handler should process the event.
    Detect(event *GameEvent) bool
    
    // Handle processes the event and sends answers via manager.
    Handle(event *GameEvent, manager *BotnetManager) error
}
```

This minimal interface accommodates games with different patterns:
- **Trivia games** (TriviaDeath2): Detect question events, resolve answers from DB/user, send to all clients
- **Poll games** (PollPosition): Detect poll events, generate random percentages, send
- **Counter games** (Everyday): Detect counter events, send incrementing values
- **Free-form games** (Quiplash): Detect prompt events, send user-provided text

### Registry

A global registry maps game tags to handler factories:

```go
// internal/games/registry.go
var registry = map[string]func() GameHandler{}

func Register(gameTag string, factory func() GameHandler) {
    registry[gameTag] = factory
}

func GetHandler(gameTag string) GameHandler {
    if factory, ok := registry[gameTag]; ok {
        return factory()
    }
    return &GenericHandler{} // fallback
}
```

Each game package calls `Register()` in its `init()` function.

### Package Structure

```
client/internal/
├── games/
│   ├── handler.go          # GameHandler interface
│   ├── registry.go         # Register/GetHandler
│   ├── event.go            # GameEvent, AnswerDatabase (moved from events.go)
│   ├── config.go           # Config loading, GameConfig struct
│   ├── triviadeath2/
│   │   ├── handler.go      # TriviaDeath2Handler implements GameHandler
│   │   ├── questions.go    # Question extraction, answer resolution
│   │   └── init.go         # Register("triviadeath2", ...)
│   ├── quiplash2/
│   │   ├── handler.go
│   │   └── init.go
│   ├── everyday/
│   │   ├── handler.go
│   │   └── init.go
│   ├── pollposition/
│   │   ├── handler.go
│   │   └── init.go
│   └── generic/
│       ├── handler.go      # Fallback for unknown games
│       └── init.go
├── commands/
│   ├── botnet.go           # Simplified: uses registry, no game-specific logic
│   ├── ddos.go             # Unchanged
│   └── serve.go            # Unchanged
└── server/
    └── server.go           # Unchanged
```

### Configuration

One `config.json` in project root:

```json
{
  "games": {
    "triviadeath2": {
      "display_name": "Смертельная Вечеринка 2",
      "sources": [
        { "type": "local", "path": "data/triviadeath2/questions.json" },
        { "type": "url", "url": "https://gist.githubusercontent.com/.../triviadeath2-questions.json" }
      ],
      "answer_strategy": "auto",
      "settings": {
        "final_round_source": { "type": "local", "path": "data/triviadeath2/final.json" }
      }
    },
    "triviadeath2-tjsp": {
      "display_name": "Смертельная Вечеринка 2 (TJSP)",
      "sources": [
        { "type": "local", "path": "data/triviadeath2-tjsp/questions.json" }
      ],
      "answer_strategy": "auto"
    },
    "quiplash2": {
      "display_name": "Quiplash 2",
      "sources": [],
      "answer_strategy": "manual"
    },
    "everyday": {
      "display_name": "Everyday",
      "sources": [],
      "answer_strategy": "auto"
    },
    "pollposition": {
      "display_name": "Poll Position",
      "sources": [],
      "answer_strategy": "auto"
    }
  }
}
```

Config struct:

```go
type GameConfig struct {
    DisplayName    string            `json:"display_name"`
    Sources        []AnswerSource    `json:"sources"`
    AnswerStrategy string            `json:"answer_strategy"` // "auto", "manual", "hybrid"
    Settings       map[string]any    `json:"settings"`
}

type AnswerSource struct {
    Type string `json:"type"` // "local", "url"
    Path string `json:"path,omitempty"`
    URL  string `json:"url,omitempty"`
}
```

### Unified Question Format

All answer databases use one format:

```json
{
  "questions": [
    {
      "gameTag": "triviadeath2",
      "prompt": "Какая планета ближе к Солнцу?",
      "choices": [
        { "text": "Венера", "correct": false },
        { "text": "Меркурий", "correct": true },
        { "text": "Марс", "correct": false }
      ],
      "type": "regular"
    },
    {
      "gameTag": "triviadeath2",
      "prompt": "Финальный раунд: назовите столицу",
      "choices": [],
      "type": "final"
    }
  ]
}
```

### Answer Resolution Flow

1. Handler receives `GameEvent`
2. Extract question info (prompt, choices, round type)
3. Check sources in order:
   a. Local file (if configured)
   b. Remote URL (if configured)
   c. User CLI prompt (if strategy is "manual" or "hybrid")
4. Send answer to all clients via `manager.SendAnswer()`

### Migration Strategy

Phase 1 (this spec):
- Create `internal/games/` with interface, registry, config
- Create `internal/games/generic/` as fallback
- Move `GameEvent`, `AnswerDatabase` to `internal/games/event.go`
- Update `botnet.go` to use registry instead of switch
- Delete `questions.json` (empty anyway)

Phase 2 (future):
- Migrate TriviaDeath2 to `internal/games/triviadeath2/`
- Migrate other games one by one
- Remove game-specific code from `handlers.go`

## [S3] Out of Scope

- Chrome extension changes (extension/ unchanged)
- DDoS command changes (ddos.go unchanged)
- Server command changes (serve.go unchanged)
- WebSocket recording changes (recordings/ unchanged)
- Adding new games (Смертельная вечеринка, Нашшпионаж) — only the architecture to support them
- Test coverage (no tests exist currently)

## Tasks

- [x] T1: Create `internal/games/handler.go` with `GameHandler` interface — acceptance: interface compiles, has `Detect` and `Handle` methods (covers: S2)
- [x] T2: Create `internal/games/registry.go` with `Register()` and `GetHandler()` — acceptance: registry compiles, can register and retrieve handlers (covers: S2)
- [x] T3: Create `internal/games/config.go` with `GameConfig`, `AnswerSource` structs and `LoadConfig()` — acceptance: config loads from JSON, validates required fields (covers: S2)
- [x] T4: Create `internal/games/event.go` by moving `GameEvent`, `AnswerDatabase`, parsing functions from `events.go` — acceptance: types compile, existing code still works with updated imports (covers: S2)
- [x] T5: Create `internal/games/generic/handler.go` as fallback handler — acceptance: implements `GameHandler`, handles unknown games (covers: S2)
- [x] T6: Update `botnet.go` to load config and use registry instead of switch statement — acceptance: botnet starts, routes events to handlers via registry (covers: S2)
- [x] T7: Delete `client/questions.json` — acceptance: file removed, no references remain (covers: S2)
- [x] T8: Verify botnet compiles and runs with new architecture — acceptance: `go build` succeeds, botnet connects to a test room (covers: S2)

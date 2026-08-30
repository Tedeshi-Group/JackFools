# Game Modules

Modular architecture for Jackbox game handlers. Each game is a separate package implementing the `GameHandler` interface.

## Quick Start

### 1. Create your game package

```
internal/games/yourgame/
└── handler.go
```

### 2. Implement the handler

```go
package yourgame

import "jackfools/client/internal/games"

func init() {
    games.Register("yourgame", func() games.GameHandler {
        return &Handler{}
    })
}

type Handler struct{}

func (h *Handler) Detect(event *games.GameEvent) bool {
    return event.GameTag == "yourgame"
}

func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
    // Your game logic here
    return nil
}
```

### 3. Register the import

Add to `client/internal/commands/botnet.go`:

```go
import (
    _ "jackfools/client/internal/games/yourgame"
)
```

### 4. Build and test

```bash
go build ./...
```

## Interface Reference

### GameHandler

```go
type GameHandler interface {
    // Detect returns true if this handler should process the event.
    // Check gameTag first (fast), then payload structure (slow).
    Detect(event *GameEvent) bool

    // Handle processes the event and sends answers via manager.
    Handle(event *GameEvent, manager BotnetManagerAPI) error
}
```

### GameEvent

```go
type GameEvent struct {
    Type           string                 // "object", "string", etc.
    EventID        string                 // Unique event ID for responses
    GameTag        string                 // "triviadeath2", "quiplash2", etc.
    Payload        map[string]interface{} // Raw event data
    RequiresAnswer bool                   // Whether this event needs a response
}
```

### BotnetManagerAPI

```go
type BotnetManagerAPI interface {
    // SendCommand sends a command to all connected audience bots.
    SendCommand(cmd ClientCommand)

    // GetAnswerDB returns the answer database for the game tag.
    GetAnswerDB(gameTag string) *AnswerDatabase

    // GetFinalRoundDB returns the final round answer database.
    GetFinalRoundDB(gameTag string) *AnswerDatabase

    // SetCurrentQuestion stores the current question for learning.
    SetCurrentQuestion(prompt string, choices []string)

    // GetGameTag/SetGameTag manage the cached game tag.
    GetGameTag() string
    SetGameTag(tag string)
}
```

### ClientCommand

```go
type ClientCommand struct {
    Type    string                 // "answer", "vote", etc.
    EventID string                 // Event ID to respond to
    Answer  string                 // Answer text or index
    Payload map[string]interface{} // Additional data
}
```

## Common Patterns

### Detecting events

```go
func (h *Handler) Detect(event *games.GameEvent) bool {
    // Fast path: gameTag match
    if event.GameTag == "yourgame" {
        return true
    }

    // Slow path: payload structure
    if event.Type == "object" && event.Payload != nil {
        if key, ok := event.Payload["key"].(string); ok {
            if key == "yourGameSpecificKey" {
                return true
            }
        }
    }

    return false
}
```

### Sending votes

```go
func (h *Handler) sendVote(eventID string, choiceIndex int, manager games.BotnetManagerAPI) {
    manager.SendCommand(games.ClientCommand{
        Type:    "answer",
        EventID: eventID,
        Answer:  fmt.Sprintf("%d", choiceIndex),
    })
}
```

### Using answer database

```go
func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
    db := manager.GetAnswerDB(event.GameTag)
    if db == nil {
        log.Printf("no answer database for %s", event.GameTag)
        return nil
    }

    // Look up answer by question text
    question := event.Payload["prompt"].(string)
    if answer, ok := db.Questions[question]; ok {
        // Use the answer
    }

    return nil
}
```

### Learning new answers

```go
func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
    // Store current question for learning
    if prompt, ok := event.Payload["prompt"].(string); ok {
        choices := extractChoices(event.Payload)
        manager.SetCurrentQuestion(prompt, choices)
    }

    return nil
}
```

## Reference Implementation

See `internal/games/fakinit/handler.go` for a complete example implementing:
- Event detection by gameTag and payload structure
- Multiple event type handling
- Auto-voting logic
- Player list extraction

## Existing Games

| Game | Package | Status |
|------|---------|--------|
| Trivia Death 2 | `commands/handlers.go` | Legacy (switch) |
| Quiplash 2 | `commands/handlers.go` | Legacy (switch) |
| Everyday | `commands/handlers.go` | Legacy (switch) |
| Poll Position | `commands/handlers.go` | Legacy (switch) |
| Fakin' It | `games/fakinit/` | Modular (reference) |

Legacy games use the switch statement in `handlers.go` as fallback. New games should use the modular system.

## Configuration

Game-specific settings can be loaded from `config.json`:

```json
{
  "games": {
    "yourgame": {
      "display_name": "Your Game Name",
      "sources": [
        { "type": "url", "url": "https://example.com/answers.json" },
        { "type": "local", "path": "answers/yourgame.json" }
      ],
      "answer_strategy": "auto",
      "settings": {
        "custom_field": "value"
      }
    }
  }
}
```

Load config in your handler:

```go
config, err := games.LoadConfig("config.json")
if err != nil {
    log.Printf("warning: failed to load config: %v", err)
}
```

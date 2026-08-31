package games

import "context"

// GameHandler is the interface that every mini-game must implement.
// Each game decides how to detect its events and how to handle them.
type GameHandler interface {
	// Detect returns true if this handler should process the given event.
	Detect(event *GameEvent) bool

	// Handle processes the event and sends answers via the manager.
	Handle(event *GameEvent, manager BotnetManagerAPI) error
}

// BotnetManagerAPI is the interface that handlers use to interact with the botnet manager.
// This decouples handlers from the concrete BotnetManager struct.
type BotnetManagerAPI interface {
	// SendCommand sends a command to all connected clients.
	SendCommand(cmd ClientCommand)

	// GetAnswerDB returns the answer database for the given game tag.
	GetAnswerDB(gameTag string) *AnswerDatabase

	// GetFinalRoundDB returns the final round answer database for the given game tag.
	GetFinalRoundDB(gameTag string) *AnswerDatabase

	// SetCurrentQuestion sets the current question for learning.
	SetCurrentQuestion(prompt string, choices []string)

	// GetGameTag returns the cached game tag.
	GetGameTag() string

	// SetGameTag sets the cached game tag.
	SetGameTag(tag string)

	// GetContext returns the context for cancellation.
	GetContext() context.Context
}

// ClientCommand represents a command sent from coordinator to clients.
type ClientCommand struct {
	Type    string                 // Command type (e.g., "answer").
	EventID string                 // Event ID to respond to.
	Answer  string                 // Answer to send.
	Payload map[string]interface{} // Additional command data.
}

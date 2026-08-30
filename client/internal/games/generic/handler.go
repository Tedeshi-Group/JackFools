package generic

import (
	"jackfools/client/internal/games"
	"log"
)

func init() {
	games.Register("generic", func() games.GameHandler {
		return &Handler{}
	})
}

// Handler is the fallback handler for unknown games.
type Handler struct{}

// Detect returns false so the fallback to legacy switch statement works.
func (h *Handler) Detect(event *games.GameEvent) bool {
	return false
}

// Handle logs the event and does nothing.
func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
	log.Printf("generic handler: unhandled event type=%s, gameTag=%s, eventID=%s",
		event.Type, event.GameTag, event.EventID)
	return nil
}

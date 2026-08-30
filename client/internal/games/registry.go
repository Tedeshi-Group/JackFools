package games

import "log"

// registry maps game tags to handler factories.
var registry = map[string]func() GameHandler{}

// Register adds a handler factory for the given game tag.
// Each game package should call this in its init() function.
func Register(gameTag string, factory func() GameHandler) {
	if _, exists := registry[gameTag]; exists {
		log.Printf("warning: overwriting handler for game tag %s", gameTag)
	}
	registry[gameTag] = factory
}

// GetHandler returns a new handler instance for the given game tag.
// Returns a noOpHandler if no specific handler is registered.
func GetHandler(gameTag string) GameHandler {
	if factory, ok := registry[gameTag]; ok {
		return factory()
	}
	return &noOpHandler{}
}

// noOpHandler is a handler that does nothing and never matches.
type noOpHandler struct{}

func (h *noOpHandler) Detect(event *GameEvent) bool { return false }
func (h *noOpHandler) Handle(event *GameEvent, manager BotnetManagerAPI) error { return nil }

// RegisteredGames returns a list of all registered game tags.
func RegisteredGames() []string {
	tags := make([]string, 0, len(registry))
	for tag := range registry {
		tags = append(tags, tag)
	}
	return tags
}

// Package fakinit implements the audience bot handler for "Fakin' It" (Нашшпионаж).
//
// This is a reference implementation showing how to create a modular game handler.
// Use this as a template when adding support for new Jackbox games.
//
// Game mechanics (audience mode):
//   - Audience sees a prompt asking to identify the faker
//   - Audience votes for one of the players
//   - Votes are tallied and the player with most votes is revealed
//
// To add a new game:
//  1. Copy this package to internal/games/yourgame/
//  2. Update the package name and Register() call
//  3. Implement Detect() to match your game's events
//  4. Implement Handle() with your game logic
//  5. Import the package in botnet.go with: _ "jackfools/client/internal/games/yourgame"
package fakinit

import (
	"fmt"
	"log"
	"math/rand"

	"jackfools/client/internal/games"
)

// Game tags for Fakin' It variants.
const (
	GameTagFakinIt = "fakinit"
)

func init() {
	games.Register(GameTagFakinIt, func() games.GameHandler {
		return &Handler{
			players: make([]string, 0),
		}
	})
}

// Handler implements games.GameHandler for Fakin' It audience mode.
type Handler struct {
	players       []string // Current player names.
	currentPrompt string   // Current question prompt.
}

// Detect returns true if the event is a Fakin' It audience event.
//
// Detection strategy:
//   - Check gameTag first (fast path)
//   - Fall back to payload structure analysis (slow path)
//
// Adapt this to your game's event structure.
func (h *Handler) Detect(event *games.GameEvent) bool {
	// Fast path: gameTag matches.
	if event.GameTag == GameTagFakinIt {
		return true
	}

	// Slow path: detect by payload structure.
	// Fakin' It audience events have specific fields in their payload.
	if event.Type == "object" && event.Payload != nil {
		if key, ok := event.Payload["key"].(string); ok {
			// Fakin' It uses "audiencePlayer" key for audience prompts.
			if key == "audiencePlayer" {
				return true
			}
		}
	}

	return false
}

// Handle processes a Fakin' It event and sends auto-votes.
//
// Event types handled:
//   - audiencePlayer: New prompt for audience → store prompt, extract player list
//   - audience/count-group/increment: Vote submission → send auto-vote
//
// Adapt the event types and payload parsing to your game.
func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
	log.Printf("fakinit: processing event type=%s, eventID=%s", event.Type, event.EventID)

	switch event.Type {
	case "object":
		return h.handleObject(event, manager)
	default:
		log.Printf("fakinit: ignoring event type=%s", event.Type)
		return nil
	}
}

// handleObject processes object-type events (state changes, prompts, etc.).
func (h *Handler) handleObject(event *games.GameEvent, manager games.BotnetManagerAPI) error {
	key, _ := event.Payload["key"].(string)
	val, _ := event.Payload["val"].(map[string]interface{})

	switch key {
	case "audiencePlayer":
		return h.handleAudiencePrompt(event, val, manager)
	case "bc:room":
		return h.handleRoomState(event, val, manager)
	default:
		log.Printf("fakinit: ignoring object key=%s", key)
		return nil
	}
}

// handleAudiencePrompt processes a new audience prompt.
// Extracts the question text and available choices.
func (h *Handler) handleAudiencePrompt(event *games.GameEvent, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	// Extract prompt text.
	if prompt, ok := val["prompt"].(string); ok {
		h.currentPrompt = prompt
		log.Printf("fakinit: new prompt: %s", prompt)
	}

	// Extract player list for voting.
	if choices, ok := val["choices"].([]interface{}); ok {
		h.players = make([]string, 0, len(choices))
		for _, c := range choices {
			if choice, ok := c.(map[string]interface{}); ok {
				if name, ok := choice["text"].(string); ok {
					h.players = append(h.players, name)
				}
			}
		}
		log.Printf("fakinit: players available: %v", h.players)
	}

	// If this prompt requires a vote, send auto-vote.
	if hasSubmit, ok := val["hasSubmit"].(bool); ok && hasSubmit {
		return h.sendVote(event.EventID, manager)
	}

	return nil
}

// handleRoomState processes room state changes.
func (h *Handler) handleRoomState(event *games.GameEvent, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	// Track game state changes if needed.
	if state, ok := val["state"].(string); ok {
		log.Printf("fakinit: room state changed to %s", state)
	}
	return nil
}

// sendVote sends an auto-vote for a random player.
//
// Vote format: index of the chosen player as string.
// Adapt the vote format to your game's expected protocol.
func (h *Handler) sendVote(eventID string, manager games.BotnetManagerAPI) error {
	if len(h.players) == 0 {
		return fmt.Errorf("no players available to vote for")
	}

	// Pick a random player.
	chosenIndex := rand.Intn(len(h.players))
	chosenPlayer := h.players[chosenIndex]

	log.Printf("fakinit: voting for player %d (%s)", chosenIndex, chosenPlayer)

	// Send vote command to all clients.
	manager.SendCommand(games.ClientCommand{
		Type:    "answer",
		EventID: eventID,
		Answer:  fmt.Sprintf("%d", chosenIndex),
	})

	return nil
}

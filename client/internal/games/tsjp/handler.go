// Package tsjp implements the game handler for Trivia Death 2 TJSP (TSJP).
//
// This game has two roles:
//   - Audience: votes on trivia questions, predicts who dies, final round voting
//   - Player: answers trivia questions, plays minigames (math, swords, chalices), final round
//
// Audience wire format (via audience/count-group/increment):
//
//	Regular:  { name: "TriviaDeath2AudienceChoice", vote: "<index>", times: 1 }
//	Final:    { name: "TriviaDeath2AudienceChoice", vote: "0,1,2",  times: 1 }
//
// Player wire format (via object/update):
//
//	Choice:   { key: "choose:N",    val: { action: "submit", choice: <index> } }
//	Grid:     { key: "grid:N",      val: { action: "select", x: <col>, y: <row> } }
//	Final:    { key: "finalround:N", val: { action: "select"|"unselect"|"submit", choice: <index> } }
package tsjp

import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"jackfools/client/internal/games"
)

// GameTag is the game identifier for Trivia Death 2 TJSP.
const GameTag = "triviadeath2-tjsp"

func init() {
	games.Register(GameTag, func() games.GameHandler {
		return &Handler{}
	})
}

// Handler implements games.GameHandler for TSJP.
type Handler struct{}

// Detect returns true if this handler should process the event.
func (h *Handler) Detect(event *games.GameEvent) bool {
	if event.GameTag == GameTag || strings.Contains(event.GameTag, "triviadeath2") {
		return true
	}

	if event.Type == "object" && event.Payload != nil {
		if key, ok := event.Payload["key"].(string); ok {
			if key == "audiencePlayer" || strings.HasPrefix(key, "player:") {
				return true
			}
		}
	}

	return false
}

// Handle processes a TSJP event and sends auto-responses.
func (h *Handler) Handle(event *games.GameEvent, manager games.BotnetManagerAPI) error {
	switch event.Type {
	case "object":
		return h.handleObject(event, manager)
	default:
		return nil
	}
}

// handleObject dispatches object-type events by key.
func (h *Handler) handleObject(event *games.GameEvent, manager games.BotnetManagerAPI) error {
	key, _ := event.Payload["key"].(string)
	val, _ := event.Payload["val"].(map[string]interface{})

	switch {
	case key == "audiencePlayer":
		return h.handleAudienceEvent(event, val, manager)
	case strings.HasPrefix(key, "player:"):
		playerID := strings.TrimPrefix(key, "player:")
		return h.handlePlayerEvent(event, playerID, val, manager)
	default:
		return nil
	}
}

// ─── Audience ───────────────────────────────────────────────────────────────

func (h *Handler) handleAudienceEvent(event *games.GameEvent, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	roundType, _ := val["roundType"].(string)

	if roundType == "FinalRound" {
		return h.handleAudienceFinalRound(event, val, manager)
	}

	hasSubmit, _ := val["hasSubmit"].(bool)
	if hasSubmit {
		return nil
	}

	prompt, _ := val["prompt"].(string)
	choices := extractChoiceTexts(val)
	if prompt == "" || len(choices) == 0 {
		return nil
	}

	if isWhoDiesQuestion(prompt) {
		return h.handleWhoDiesVote(event.EventID, choices, manager)
	}

	// Store question for learning.
	manager.SetCurrentQuestion(prompt, choices)

	return h.handleAudienceQuestion(event.EventID, prompt, choices, manager)
}

func (h *Handler) handleAudienceQuestion(eventID, prompt string, choices []string, manager games.BotnetManagerAPI) error {
	db := manager.GetAnswerDB(GameTag)
	if db != nil {
		normalized := normalizeQuestionText(prompt)
		if answer, ok := db.Questions[normalized]; ok {
			if idx := matchAnswerIndex(answer, choices); idx >= 0 {
				return sendAudienceVote(eventID, idx, manager)
			}
		}
	}

	return sendAudienceVote(eventID, rand.Intn(len(choices)), manager)
}

func (h *Handler) handleWhoDiesVote(eventID string, choices []string, manager games.BotnetManagerAPI) error {
	valid := []int{}
	for i, c := range choices {
		upper := strings.ToUpper(c)
		if upper != "НИКТО" && upper != "NOBODY" && upper != "NO ONE" {
			valid = append(valid, i)
		}
	}

	if len(valid) == 0 {
		return sendAudienceVote(eventID, rand.Intn(len(choices)), manager)
	}

	return sendAudienceVote(eventID, valid[rand.Intn(len(valid))], manager)
}

func (h *Handler) handleAudienceFinalRound(event *games.GameEvent, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	prompt, _ := val["prompt"].(string)
	choices := extractChoiceTexts(val)
	if prompt == "" || len(choices) == 0 {
		return nil
	}

	manager.SetCurrentQuestion(prompt, choices)

	db := manager.GetFinalRoundDB(GameTag)
	if db != nil {
		normalized := normalizeQuestionText(prompt)
		if correctTexts, ok := db.FinalRoundQuestions[normalized]; ok {
			indices := matchMultiAnswerIndices(correctTexts, choices)
			if len(indices) > 0 {
				return sendAudienceFinalVote(event.EventID, indices, manager)
			}
		}
	}

	return sendAudienceFinalVote(event.EventID, []int{0}, manager)
}

// ─── Player ─────────────────────────────────────────────────────────────────

func (h *Handler) handlePlayerEvent(event *games.GameEvent, playerID string, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	kind, _ := val["kind"].(string)

	switch kind {
	case "choices":
		return h.handlePlayerChoices(event, playerID, val, manager)
	case "gridSelecting":
		return h.handlePlayerGrid(event, playerID, val, manager)
	default:
		log.Printf("tsjp: unhandled player kind=%s", kind)
		return nil
	}
}

func (h *Handler) handlePlayerChoices(event *games.GameEvent, playerID string, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	responseKey, _ := val["responseKey"].(string)
	prompt, _ := val["prompt"].(string)
	choices := extractChoiceTexts(val)

	if responseKey == "" || prompt == "" || len(choices) == 0 {
		return nil
	}

	// Final round: hasSubmit == true, responseKey starts with "finalround:".
	if strings.HasPrefix(responseKey, "finalround:") {
		return h.handlePlayerFinalRound(responseKey, prompt, choices, manager)
	}

	// Chalice minigame: pick random cup.
	if category, _ := val["category"].(string); category == "chalices" {
		return sendPlayerChoice(responseKey, rand.Intn(len(choices)), manager)
	}

	// Frozen: no choices available (server sent empty array with "Ты заморожен!").
	if len(choices) == 0 {
		return nil
	}

	// Math minigame: prompt is a math expression like "9-9", "7+8".
	if isMathExpression(prompt) {
		if correctIdx := solveMath(prompt, choices); correctIdx >= 0 {
			return sendPlayerChoice(responseKey, correctIdx, manager)
		}
		// Can't solve: pick random.
		return sendPlayerChoice(responseKey, rand.Intn(len(choices)), manager)
	}

	// Regular question: try DB lookup.
	manager.SetCurrentQuestion(prompt, choices)

	db := manager.GetAnswerDB(GameTag)
	if db != nil {
		normalized := normalizeQuestionText(prompt)
		if answer, ok := db.Questions[normalized]; ok {
			if idx := matchAnswerIndex(answer, choices); idx >= 0 {
				return sendPlayerChoice(responseKey, idx, manager)
			}
		}
	}

	return sendPlayerChoice(responseKey, rand.Intn(len(choices)), manager)
}

func (h *Handler) handlePlayerGrid(event *games.GameEvent, playerID string, val map[string]interface{}, manager games.BotnetManagerAPI) error {
	responseKey, _ := val["responseKey"].(string)
	if responseKey == "" {
		return nil
	}

	grid, _ := val["grid"].([]interface{})
	if len(grid) == 0 {
		return nil
	}

	// Pick a random cell.
	rows := len(grid)
	cols := 0
	if firstRow, ok := grid[0].([]interface{}); ok {
		cols = len(firstRow)
	}
	if cols == 0 {
		return nil
	}

	x := rand.Intn(cols)
	y := rand.Intn(rows)

	return sendPlayerGrid(responseKey, x, y, manager)
}

func (h *Handler) handlePlayerFinalRound(responseKey, prompt string, choices []string, manager games.BotnetManagerAPI) error {
	manager.SetCurrentQuestion(prompt, choices)

	db := manager.GetFinalRoundDB(GameTag)
	if db != nil {
		normalized := normalizeQuestionText(prompt)
		if correctTexts, ok := db.FinalRoundQuestions[normalized]; ok {
			indices := matchMultiAnswerIndices(correctTexts, choices)
			if len(indices) > 0 {
				// Select each correct answer, then submit.
				for _, idx := range indices {
					if err := sendPlayerFinalRoundSelect(responseKey, idx, manager); err != nil {
						return err
					}
				}
				return sendPlayerFinalRoundSubmit(responseKey, manager)
			}
		}
	}

	// No DB match: select first choice and submit.
	if err := sendPlayerFinalRoundSelect(responseKey, 0, manager); err != nil {
		return err
	}
	return sendPlayerFinalRoundSubmit(responseKey, manager)
}

// ─── Send helpers ───────────────────────────────────────────────────────────

func sendAudienceVote(eventID string, index int, manager games.BotnetManagerAPI) error {
	manager.SendCommand(games.ClientCommand{
		Type:    "answer",
		EventID: eventID,
		Answer:  fmt.Sprintf("%d", index),
		Payload: map[string]interface{}{
			"gameTag":     GameTag,
			"answerIndex": index,
		},
	})
	return nil
}

func sendAudienceFinalVote(eventID string, indices []int, manager games.BotnetManagerAPI) error {
	parts := make([]string, len(indices))
	for i, idx := range indices {
		parts[i] = fmt.Sprintf("%d", idx)
	}

	manager.SendCommand(games.ClientCommand{
		Type:    "answer",
		EventID: eventID,
		Answer:  strings.Join(parts, ","),
		Payload: map[string]interface{}{
			"gameTag":       GameTag,
			"isFinalRound":  true,
			"answerIndices": indices,
		},
	})
	return nil
}

func sendPlayerChoice(responseKey string, choice int, manager games.BotnetManagerAPI) error {
	manager.SendCommand(games.ClientCommand{
		Type: "answer",
		Payload: map[string]interface{}{
			"gameTag":     GameTag,
			"playerMode":  true,
			"responseKey": responseKey,
			"action":      "submit",
			"choice":      choice,
		},
	})
	return nil
}

func sendPlayerGrid(responseKey string, x, y int, manager games.BotnetManagerAPI) error {
	manager.SendCommand(games.ClientCommand{
		Type: "answer",
		Payload: map[string]interface{}{
			"gameTag":     GameTag,
			"playerMode":  true,
			"responseKey": responseKey,
			"action":      "select_grid",
			"x":           x,
			"y":           y,
		},
	})
	return nil
}

func sendPlayerFinalRoundSelect(responseKey string, choice int, manager games.BotnetManagerAPI) error {
	manager.SendCommand(games.ClientCommand{
		Type: "answer",
		Payload: map[string]interface{}{
			"gameTag":     GameTag,
			"playerMode":  true,
			"responseKey": responseKey,
			"action":      "select",
			"choice":      choice,
		},
	})
	return nil
}

func sendPlayerFinalRoundSubmit(responseKey string, manager games.BotnetManagerAPI) error {
	manager.SendCommand(games.ClientCommand{
		Type: "answer",
		Payload: map[string]interface{}{
			"gameTag":     GameTag,
			"playerMode":  true,
			"responseKey": responseKey,
			"action":      "submit",
		},
	})
	return nil
}

// ─── Utilities ──────────────────────────────────────────────────────────────

func extractChoiceTexts(val map[string]interface{}) []string {
	out := []string{}
	if raw, ok := val["choices"].([]interface{}); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					out = append(out, text)
				}
			}
		}
	}
	return out
}

func matchAnswerIndex(answer interface{}, choices []string) int {
	answerStr := fmt.Sprintf("%v", answer)
	if idx, err := strconv.Atoi(answerStr); err == nil {
		if idx >= 0 && idx < len(choices) {
			return idx
		}
	}

	normalizedAnswer := normalizeAnswerText(answerStr)
	for i, choice := range choices {
		if normalizeAnswerText(choice) == normalizedAnswer {
			return i
		}
	}
	return -1
}

func matchMultiAnswerIndices(correctTexts []string, choices []string) []int {
	indices := []int{}
	for i, choice := range choices {
		nc := normalizeAnswerText(choice)
		for _, ct := range correctTexts {
			if nc == ct {
				indices = append(indices, i)
				break
			}
		}
	}
	return indices
}

func isWhoDiesQuestion(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "умрёт") ||
		strings.Contains(lower, "умрет") ||
		strings.Contains(lower, "кто умр") ||
		strings.Contains(lower, "who will die")
}

// ─── Math solver ────────────────────────────────────────────────────────────

var mathExprRe = regexp.MustCompile(`^(\d+)\s*([+\-×*÷/])\s*(\d+)$`)

func isMathExpression(s string) bool {
	return mathExprRe.MatchString(strings.TrimSpace(s))
}

func solveMath(expr string, choices []string) int {
	m := mathExprRe.FindStringSubmatch(strings.TrimSpace(expr))
	if m == nil {
		return -1
	}

	a, _ := strconv.Atoi(m[1])
	op := m[2]
	b, _ := strconv.Atoi(m[3])

	var result int
	switch op {
	case "+":
		result = a + b
	case "-":
		result = a - b
	case "×", "*":
		result = a * b
	case "÷", "/":
		if b != 0 {
			result = a / b
		}
	}

	resultStr := strconv.Itoa(result)
	for i, choice := range choices {
		if choice == resultStr {
			return i
		}
	}
	return -1
}

// ─── Text normalization ─────────────────────────────────────────────────────

func normalizeQuestionText(text string) string {
	return games.NormalizeQuestionText(text)
}

func normalizeAnswerText(text string) string {
	return games.NormalizeAnswerText(text)
}

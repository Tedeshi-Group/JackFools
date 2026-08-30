package games

// GameEvent represents a typed game event.
type GameEvent struct {
	Type           string                 // Event type (e.g., "question", "answer_choice").
	EventID        string                 // Unique event ID.
	GameTag        string                 // Game tag (e.g., "quiplash2").
	Payload        map[string]interface{} // Event payload.
	RequiresAnswer bool                   // Whether this event requires an answer.
}

// AnswerDatabase represents a database of correct answers.
// Supports multiple formats:
// 1. Standard: { "games": { "gameTag": { "eventTypes": { ... } } } }
// 2. Direct: { "question": "answer" }
// 3. Content format: { "content": [ { "text": "question", "choices": [...] } ] }
type AnswerDatabase struct {
	Games               map[string]GameAnswers  `json:"games"`
	Questions           map[string]interface{}  `json:"-"`
	FinalRoundQuestions map[string][]string     `json:"-"`
}

// GameAnswers represents answers for a specific game.
type GameAnswers struct {
	EventTypes map[string]EventAnswers `json:"eventTypes"`
}

// EventAnswers represents answers for a specific event type.
type EventAnswers struct {
	Answers map[string]string `json:"answers"`
}

// TriviaDeath2QuestionItem represents a question item from the content format.
type TriviaDeath2QuestionItem struct {
	Text    string               `json:"text"`
	ID      string               `json:"id"`
	Choices []TriviaDeath2Choice `json:"choices"`
}

// TriviaDeath2Choice represents an answer choice.
type TriviaDeath2Choice struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

// TriviaDeath2ContentFormat represents the content array format.
type TriviaDeath2ContentFormat struct {
	Content []TriviaDeath2QuestionItem `json:"content"`
}

// QuestionInfo represents extracted question information.
// This is a generic struct that handlers can use to pass question data.
type QuestionInfo struct {
	Prompt    string   // Question text.
	Choices   []string // Answer choices.
	RoundType string   // Round type (e.g., "regular", "final").
	Extra     map[string]interface{} // Game-specific extra data.
}

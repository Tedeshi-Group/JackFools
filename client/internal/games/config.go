package games

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the root configuration file.
type Config struct {
	Games map[string]GameConfig `json:"games"`
}

// GameConfig represents configuration for a single game.
type GameConfig struct {
	DisplayName    string         `json:"display_name"`
	Sources        []AnswerSource `json:"sources"`
	AnswerStrategy string         `json:"answer_strategy"` // "auto", "manual", "hybrid"
	Settings       map[string]any `json:"settings"`
}

// AnswerSource represents a source for answer data.
type AnswerSource struct {
	Type string `json:"type"` // "local", "url"
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

// LoadConfig loads the configuration from the given file path.
// Returns an error if the file cannot be read or parsed.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.Games == nil {
		config.Games = make(map[string]GameConfig)
	}

	return &config, nil
}

// GetGameConfig returns the configuration for the given game tag.
// Returns nil if the game is not configured.
func (c *Config) GetGameConfig(gameTag string) *GameConfig {
	if gc, ok := c.Games[gameTag]; ok {
		return &gc
	}
	return nil
}

// GetAnswerStrategy returns the answer strategy for the given game tag.
// Defaults to "auto" if not configured.
func (c *Config) GetAnswerStrategy(gameTag string) string {
	if gc := c.GetGameConfig(gameTag); gc != nil {
		if gc.AnswerStrategy != "" {
			return gc.AnswerStrategy
		}
	}
	return "auto"
}

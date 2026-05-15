// Package theme provides a structured way to manage dark and light color styles
package theme

import (
	"encoding/json"
	"fmt"
	"io"
)

// Theme holds the styles for all UI elements, keyed by a style identifier.
type Theme[TStyle ~string] struct {
	Styles map[TStyle]Colors `json:"styles"`
}

// NewTheme creates a new Theme with the provided styles. The styles map should
// contain entries for all style identifiers used in the application, each with valid dark and/or light styles.
func NewTheme[TStyle ~string](styles map[TStyle]Colors) (Theme[TStyle], error) {
	for key, style := range styles {
		if err := style.Validate(); err != nil {
			return Theme[TStyle]{}, fmt.Errorf("%w: key %v", err, key)
		}
	}
	return Theme[TStyle]{Styles: styles}, nil
}

// Resolve returns the color for the given style key and dark-mode flag.
// If the key does not exist it returns the zero ColorStr.
func (t Theme[TStyle]) Resolve(key TStyle, isDarkMode bool) ColorStr {
	colors, ok := t.Styles[key]
	if !ok {
		return ColorStr("")
	}
	c, _ := colors.Get(isDarkMode).(ColorStr)
	return c
}

// LoadTheme reads a JSON-encoded theme from the provided io.Reader and returns a Theme instance.
// The JSON should be structured as a map of style identifiers to their corresponding Colors.
func LoadTheme[TStyle ~string](input io.Reader) (*Theme[TStyle], error) {
	var theme Theme[TStyle]
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&theme); err != nil {
		return nil, fmt.Errorf("failed to decode theme: %w", err)
	}

	// Validate each style in the theme to ensure they have at least one valid color.
	for key, style := range theme.Styles {
		if err := style.Validate(); err != nil {
			return nil, fmt.Errorf("%w: key %v", err, key)
		}
	}

	return &theme, nil
}

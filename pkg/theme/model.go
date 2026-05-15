package theme

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// ErrInvalidStyle is returned when a Colors has both Dark and Light set to nil.
var ErrInvalidStyle = fmt.Errorf("invalid style: both Dark and Light are nil")

// ColorStr is a color string (e.g. hex "#FF0000" or ANSI256 value) that satisfies color.Color via lipgloss.
type ColorStr string

// RGBA converts the ColorStr to its RGBA representation. It uses lipgloss's color parsing
func (c ColorStr) RGBA() (r, g, b, a uint32) {
	return lipgloss.Color(string(c)).RGBA()
}

// AsColor returns the ColorStr as a color.Color.
func (c *ColorStr) AsColor() color.Color {
	return c
}

// Colors holds optional dark and light variants of a single UI color.
type Colors struct {
	Dark  *ColorStr `json:"dark,omitempty"`
	Light *ColorStr `json:"light,omitempty"`
}

// Validate returns ErrInvalidStyle when both Dark and Light are nil.
func (c Colors) Validate() error {
	if c.Dark == nil && c.Light == nil {
		return ErrInvalidStyle
	}
	return nil
}

// Get returns the appropriate lipgloss style based on the isDarkMode flag. If
// the requested mode's style is nil, it falls back to the other mode's style if
// available, or an empty style if neither is set.
func (c Colors) Get(isDarkMode bool) color.Color {
	var preferred, fallback *ColorStr
	if isDarkMode {
		preferred, fallback = c.Dark, c.Light
	} else {
		preferred, fallback = c.Light, c.Dark
	}
	return or(preferred, fallback, lazyfy(ColorStr("Black")))
}

func lazyfy[T any](v T) func() T {
	return func() T { return v }
}

func or[T any](a, b *T, fallback func() T) T {
	if a != nil {
		return *a
	}
	if b != nil {
		return *b
	}
	return fallback()
}

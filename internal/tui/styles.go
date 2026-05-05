package tui

import "charm.land/lipgloss/v2"

// Styles holds all lipgloss styles used by mrboard widgets.
type Styles struct {
	ColumnHeader        lipgloss.Style
	ColumnBorder        lipgloss.Style
	ColumnBorderFocused lipgloss.Style
	Card                lipgloss.Style
	CardFocused         lipgloss.Style
	CardTitle           lipgloss.Style
	CardAuthor          lipgloss.Style
	CardMeta            lipgloss.Style
	PillNotStarted      lipgloss.Style
	PillCommented       lipgloss.Style
	PillReReview        lipgloss.Style
	PillApproved        lipgloss.Style
	DurationUrgent      lipgloss.Style
	DurationWarning     lipgloss.Style
	DurationOk          lipgloss.Style
	EmptyColumn         lipgloss.Style
	ErrorMsg            lipgloss.Style
}

// NewStyles creates and returns the default mrboard styles.
func NewStyles() Styles {
	return Styles{
		ColumnHeader:        lipgloss.NewStyle().Bold(true).Padding(0, 1),
		ColumnBorder:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")),
		ColumnBorderFocused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")),
		Card:                lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		CardFocused:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")).Background(lipgloss.Color("236")).Padding(0, 1),
		CardTitle:           lipgloss.NewStyle().Bold(true),
		CardAuthor:          lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true),
		CardMeta:            lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		PillNotStarted:      lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		PillCommented:       lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		PillReReview:        lipgloss.NewStyle().Foreground(lipgloss.Color("99")),
		PillApproved:        lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		DurationUrgent:      lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		DurationWarning:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		DurationOk:          lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		EmptyColumn:         lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true),
		ErrorMsg:            lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
	}
}

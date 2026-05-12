package tui

import "charm.land/lipgloss/v2"

// Styles holds all lipgloss styles used by mrboard widgets.
type Styles struct {
	Header                      lipgloss.Style
	HeaderTitle                 lipgloss.Style
	HeaderStats                 lipgloss.Style
	Footer                      lipgloss.Style
	ColumnHeader                lipgloss.Style
	ColumnBorder                lipgloss.Style
	ColumnBorderFocused         lipgloss.Style
	ColumnBorderFocusedInactive lipgloss.Style
	Card                        lipgloss.Style
	CardFocused                 lipgloss.Style
	CardFocusedInactive         lipgloss.Style
	CardTitle                   lipgloss.Style
	CardAuthor                  lipgloss.Style
	CardMeta                    lipgloss.Style
	PillNotStarted              lipgloss.Style
	PillCommented               lipgloss.Style
	PillReReview                lipgloss.Style
	PillApproved                lipgloss.Style
	DurationUrgent              lipgloss.Style
	DurationWarning             lipgloss.Style
	DurationOk                  lipgloss.Style
	EmptyColumn                 lipgloss.Style
	ErrorMsg                    lipgloss.Style
	ScrollIndicator             lipgloss.Style
	DetailPanel                 lipgloss.Style
	DetailTitle                 lipgloss.Style
	DetailSectionHeader         lipgloss.Style
	DetailBody                  lipgloss.Style
	DetailMeta                  lipgloss.Style
	MRNumberBang                lipgloss.Style // colored "!" sigil in !IID refs
	PopupBorder                 lipgloss.Style
	PopupTitle                  lipgloss.Style
	PopupSection                lipgloss.Style
	PopupItem                   lipgloss.Style
	PopupItemFocused            lipgloss.Style
	PopupHint                   lipgloss.Style
	FilterActive                lipgloss.Style
	FooterVersion               lipgloss.Style
}

// NewStyles creates and returns the default mrboard styles.
func NewStyles() Styles {
	return Styles{
		Header: lipgloss.NewStyle().
			Background(lipgloss.Color("235")),
		HeaderTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true),
		HeaderStats: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
		ColumnHeader: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		ColumnBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")),
		ColumnBorderFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")),
		ColumnBorderFocusedInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")),
		Card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
		CardFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),
		CardFocusedInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("60")).
			Background(lipgloss.Color("235")).
			Padding(0, 1),
		CardTitle:       lipgloss.NewStyle().Bold(true),
		CardAuthor:      lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true),
		CardMeta:        lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		PillNotStarted:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		PillCommented:   lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		PillReReview:    lipgloss.NewStyle().Foreground(lipgloss.Color("99")),
		PillApproved:    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		DurationUrgent:  lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		DurationWarning: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		DurationOk:      lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		EmptyColumn:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true),
		ErrorMsg:        lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		ScrollIndicator: lipgloss.NewStyle().Foreground(lipgloss.Color("99")),
		DetailPanel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(0, 1),
		DetailTitle:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")),
		DetailSectionHeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")),
		DetailBody:          lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		DetailMeta:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		MRNumberBang:        lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true),
		PopupBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(0, 1),
		PopupTitle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")),
		PopupSection:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")),
		PopupItem:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		PopupItemFocused: lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236")).Bold(true),
		PopupHint:        lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		FilterActive:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		FooterVersion:    lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
	}
}

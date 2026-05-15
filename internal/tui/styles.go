package tui

import (
	"image/color"

	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/pkg/theme"
)

// Styles holds all lipgloss styles used by mrboard widgets.
type Styles struct {
	Card                        lip.Style
	CardAuthor                  lip.Style
	CardFocused                 lip.Style
	CardFocusedInactive         lip.Style
	CardMeta                    lip.Style
	CardTitle                   lip.Style
	ColumnBorder                lip.Style
	ColumnBorderFocused         lip.Style
	ColumnBorderFocusedInactive lip.Style
	ColumnHeader                lip.Style
	DetailBody                  lip.Style
	DetailMeta                  lip.Style
	DetailPanel                 lip.Style
	DetailSectionHeader         lip.Style
	DetailTitle                 lip.Style
	DurationOk                  lip.Style
	DurationUrgent              lip.Style
	DurationWarning             lip.Style
	EmptyColumn                 lip.Style
	ErrorMsg                    lip.Style
	FilterActive                lip.Style
	Footer                      lip.Style
	FooterVersion               lip.Style
	Header                      lip.Style
	HeaderStats                 lip.Style
	HeaderTitle                 lip.Style
	MRNumberBang                lip.Style
	PillApproved                lip.Style
	PillCommented               lip.Style
	PillNotStarted              lip.Style
	PillReReview                lip.Style
	PopupBorder                 lip.Style
	PopupHint                   lip.Style
	PopupItem                   lip.Style
	PopupItemFocused            lip.Style
	PopupSection                lip.Style
	PopupTitle                  lip.Style
	ScrollIndicator             lip.Style
	UsernameAtSign              lip.Style
}

// NewStyles builds the full style set from the given theme and dark-mode flag.
func NewStyles(th theme.Theme[ColorKey], isDark bool) Styles {
	c := func(key ColorKey) color.Color {
		return lip.Color(string(th.Resolve(key, isDark)))
	}

	return Styles{
		Header: lip.NewStyle().
			Background(c(BgBase)),
		HeaderTitle: lip.NewStyle().
			Foreground(c(Accent)).
			Bold(true),
		HeaderStats: lip.NewStyle().
			Foreground(c(FgMedium)),
		Footer: lip.NewStyle().
			Foreground(c(FgLow)),
		FooterVersion: lip.NewStyle().
			Foreground(c(FgLow)),
		ColumnHeader: lip.NewStyle().Bold(true).Padding(0, 1),
		ColumnBorder: lip.NewStyle().
			Border(lip.RoundedBorder()).BorderForeground(c(Border)),
		ColumnBorderFocused: lip.NewStyle().
			Border(lip.RoundedBorder()).BorderForeground(c(BorderFocus)),
		ColumnBorderFocusedInactive: lip.NewStyle().
			Border(lip.RoundedBorder()).BorderForeground(c(Border)),
		Card: lip.NewStyle().
			Border(lip.RoundedBorder()).
			BorderForeground(c(Border)).
			Padding(0, 1),
		CardFocused: lip.NewStyle().
			Border(lip.RoundedBorder()).
			BorderForeground(c(BorderFocus)).
			Background(c(BgElevated)).
			Padding(0, 1),
		CardFocusedInactive: lip.NewStyle().
			Border(lip.RoundedBorder()).
			BorderForeground(c(Border)).
			Background(c(BgBase)).
			Padding(0, 1),
		CardTitle:       lip.NewStyle().Bold(true),
		CardAuthor:      lip.NewStyle().Foreground(c(Info)).Bold(true),
		CardMeta:        lip.NewStyle().Foreground(c(FgMedium)),
		PillNotStarted:  lip.NewStyle().Foreground(c(Warning)),
		PillCommented:   lip.NewStyle().Foreground(c(Info)),
		PillReReview:    lip.NewStyle().Foreground(c(Accent)),
		PillApproved:    lip.NewStyle().Foreground(c(Success)),
		DurationUrgent:  lip.NewStyle().Foreground(c(Danger)).Bold(true),
		DurationWarning: lip.NewStyle().Foreground(c(Warning)),
		DurationOk:      lip.NewStyle().Foreground(c(FgMedium)),
		EmptyColumn:     lip.NewStyle().Foreground(c(FgLow)).Italic(true),
		ErrorMsg:        lip.NewStyle().Foreground(c(Danger)).Bold(true),
		ScrollIndicator: lip.NewStyle().Foreground(c(Accent)),
		DetailPanel: lip.NewStyle().
			Border(lip.RoundedBorder()).
			BorderForeground(c(Accent)).
			Padding(0, 1),
		DetailTitle:         lip.NewStyle().Bold(true).Foreground(c(FgHigh)),
		DetailSectionHeader: lip.NewStyle().Bold(true).Foreground(c(Accent)),
		DetailBody:          lip.NewStyle().Foreground(c(FgHigh)),
		DetailMeta:          lip.NewStyle().Foreground(c(FgMedium)),
		MRNumberBang:        lip.NewStyle().Foreground(c(Accent)).Bold(true),
		PopupBorder: lip.NewStyle().
			Border(lip.RoundedBorder()).
			BorderForeground(c(Accent)).
			Padding(0, 1),
		PopupTitle:       lip.NewStyle().Bold(true).Foreground(c(Accent)),
		PopupSection:     lip.NewStyle().Bold(true).Foreground(c(FgMedium)),
		PopupItem:        lip.NewStyle().Foreground(c(FgHigh)),
		PopupItemFocused: lip.NewStyle().Foreground(c(FgHigh)).Background(c(BgElevated)).Bold(true),
		PopupHint:        lip.NewStyle().Foreground(c(FgLow)),
		FilterActive:     lip.NewStyle().Foreground(c(Warning)).Bold(true),
		UsernameAtSign:   lip.NewStyle().Foreground(c(Accent)).Bold(true),
	}
}

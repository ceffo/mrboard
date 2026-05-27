package tui

// Theme mode constants.
const (
	themeModeAuto  = "auto"
	themeModeDark  = "dark"
	themeModeLight = "light"
)

// ColorKey identifies a semantic color slot in a theme.
// These keys follow a design-first hierarchy: surface, content, chrome, brand, signal.
type ColorKey string

const (
	// BgBase is the outermost canvas background.
	BgBase ColorKey = "bg-base"
	// BgElevated is the background for cards and panels, one step above BgBase.
	BgElevated ColorKey = "bg-elevated"
	// BgOverlay is the background for popups and modals, the highest elevation layer.
	BgOverlay ColorKey = "bg-overlay"

	// FgHigh is the foreground color for titles and primary content.
	FgHigh ColorKey = "fg-high"
	// FgMedium is the foreground color for supporting text and metadata.
	FgMedium ColorKey = "fg-medium"
	// FgLow is the foreground color for hints, timestamps, and footer chrome.
	FgLow ColorKey = "fg-low"

	// Border is the color for resting structural borders.
	Border ColorKey = "border"
	// BorderFocus is the color for the active focus ring.
	BorderFocus ColorKey = "border-focus"

	// Accent is the single primary interactive and brand color.
	Accent ColorKey = "accent"

	// Success signals a positive or completed state.
	Success ColorKey = "success"
	// Warning signals a state that needs attention.
	Warning ColorKey = "warning"
	// Danger signals an error, blocking, or critical state.
	Danger ColorKey = "danger"
	// Info signals neutral informational content (cyan/teal family).
	Info ColorKey = "info"
	// ColorApprover is the color for reviewers who are designated approvers.
	ColorApprover ColorKey = "color-approver"
)

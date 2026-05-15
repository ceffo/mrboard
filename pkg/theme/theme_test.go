package theme_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/pkg/theme"
)

type styleKey string

const (
	keyPrimary   styleKey = "primary"
	keySecondary styleKey = "secondary"
	keyDarkOnly  styleKey = "dark-only"
	keyLightOnly styleKey = "light-only"
	keyMuted     styleKey = "muted"
	keyMissing   styleKey = "missing"

	colorDark  = "#111111"
	colorLight = "#EEEEEE"
)

// ---- helpers ----------------------------------------------------------------

func ptr(s string) *theme.ColorStr { c := theme.ColorStr(s); return &c }

// ---- Colors.Validate --------------------------------------------------------

func TestColors_Validate(t *testing.T) {
	cases := []struct {
		name    string
		colors  theme.Colors
		wantErr bool
	}{
		{"both set", theme.Colors{Dark: ptr("#111"), Light: ptr("#EEE")}, false},
		{"dark only", theme.Colors{Dark: ptr("#111")}, false},
		{"light only", theme.Colors{Light: ptr("#EEE")}, false},
		{"both nil", theme.Colors{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.colors.Validate()
			if tc.wantErr {
				assert.ErrorIs(t, err, theme.ErrInvalidStyle)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---- Colors.Get -------------------------------------------------------------

func TestColors_Get(t *testing.T) {
	cases := []struct {
		name       string
		colors     theme.Colors
		isDarkMode bool
		want       theme.ColorStr
	}{
		{"dark mode returns dark", theme.Colors{Dark: ptr(colorDark), Light: ptr(colorLight)}, true, colorDark},
		{"light mode returns light", theme.Colors{Dark: ptr(colorDark), Light: ptr(colorLight)}, false, colorLight},
		{"dark mode falls back to light", theme.Colors{Light: ptr(colorLight)}, true, colorLight},
		{"light mode falls back to dark", theme.Colors{Dark: ptr(colorDark)}, false, colorDark},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.colors.Get(tc.isDarkMode)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---- NewTheme ---------------------------------------------------------------

func TestNewTheme(t *testing.T) {
	cases := []struct {
		name    string
		styles  map[styleKey]theme.Colors
		wantErr bool
	}{
		{"valid both", map[styleKey]theme.Colors{keyPrimary: {Dark: ptr("#111"), Light: ptr("#EEE")}}, false},
		{"valid dark only", map[styleKey]theme.Colors{keyPrimary: {Dark: ptr("#111")}}, false},
		{"valid light only", map[styleKey]theme.Colors{keyPrimary: {Light: ptr("#EEE")}}, false},
		{"empty map", map[styleKey]theme.Colors{}, false},
		{"one invalid key", map[styleKey]theme.Colors{keyPrimary: {Dark: ptr("#111")}, "bad": {}}, true},
		{"all nil", map[styleKey]theme.Colors{"bad": {}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th, err := theme.NewTheme(tc.styles)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.styles, th.Styles)
			}
		})
	}
}

// ---- LoadTheme --------------------------------------------------------------

func TestLoadTheme_ValidFixture(t *testing.T) {
	f, err := os.Open("testdata/themes.json")
	require.NoError(t, err)
	defer f.Close()

	th, err := theme.LoadTheme[styleKey](f)
	require.NoError(t, err)

	for _, key := range []styleKey{keyPrimary, keySecondary, keyDarkOnly, keyLightOnly} {
		assert.Contains(t, th.Styles, key)
	}
}

func TestLoadTheme_Errors(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"malformed json", `{not valid json}`},
		{"invalid style both nil", `{"styles":{"bad":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := theme.LoadTheme[styleKey](strings.NewReader(tc.input))
			assert.Error(t, err)
		})
	}
}

func TestLoadTheme_Empty(t *testing.T) {
	th, err := theme.LoadTheme[styleKey](strings.NewReader(`{"styles":{}}`))
	require.NoError(t, err)
	assert.Empty(t, th.Styles)
}

// ---- Theme.Resolve ----------------------------------------------------------

func TestTheme_Resolve(t *testing.T) {
	th, err := theme.NewTheme(map[styleKey]theme.Colors{
		keyPrimary: {Dark: ptr(colorDark), Light: ptr(colorLight)},
		keyMuted:   {Dark: ptr("#AAAAAA")},
	})
	require.NoError(t, err)

	cases := []struct {
		name       string
		key        styleKey
		isDarkMode bool
		want       theme.ColorStr
	}{
		{"existing key dark mode", keyPrimary, true, colorDark},
		{"existing key light mode", keyPrimary, false, colorLight},
		{"dark-only key in light mode falls back", keyMuted, false, "#AAAAAA"},
		{"missing key returns zero value", keyMissing, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := th.Resolve(tc.key, tc.isDarkMode)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---- ColorStr ---------------------------------------------------------------

func TestColorStr_RGBA_OpaqueAlpha(t *testing.T) {
	r, g, b, a := theme.ColorStr("#FF0000").RGBA()
	assert.NotZero(t, a, "expected opaque alpha; r=%d g=%d b=%d", r, g, b)
}

func TestColorStr_AsColor_NonNil(t *testing.T) {
	c := theme.ColorStr("#00FF00")
	assert.NotNil(t, c.AsColor())
}

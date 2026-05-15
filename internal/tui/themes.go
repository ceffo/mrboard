package tui

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/pkg/theme"
)

//go:embed themes/*.json
var themesFS embed.FS

// BuiltinThemeNames returns the name of every bundled theme (filename without .json).
func BuiltinThemeNames() ([]string, error) {
	entries, err := fs.Glob(themesFS, "themes/*.json")
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = strings.TrimSuffix(strings.TrimPrefix(e, "themes/"), ".json")
	}
	return names, nil
}

// CustomThemeNames returns names of user-supplied themes from ~/.config/mrboard/themes/.
func CustomThemeNames() ([]string, error) {
	dir := customThemeDir()
	if dir == "" {
		return nil, nil
	}
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = strings.TrimSuffix(filepath.Base(e), ".json")
	}
	return names, nil
}

// AllThemeNames returns all theme names (custom + builtin) sorted alphabetically.
// A custom theme with the same name as a builtin overrides the builtin.
func AllThemeNames() ([]string, error) {
	builtins, err := BuiltinThemeNames()
	if err != nil {
		return nil, err
	}
	customs, err := CustomThemeNames()
	if err != nil {
		slog.Default().Warn("theme: list custom themes", "err", err)
	}

	seen := make(map[string]bool)
	var names []string
	for _, n := range customs {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, n := range builtins {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

// LoadThemeByName loads a theme by name, checking the custom dir first then builtins.
// Falls back to "default" on any error.
func LoadThemeByName(name string) theme.Theme[ColorKey] {
	if name == "" {
		name = "default"
	}

	if dir := customThemeDir(); dir != "" {
		path := filepath.Join(dir, name+".json")
		if f, ferr := os.Open(path); ferr == nil {
			defer f.Close()
			th, terr := theme.LoadTheme[ColorKey](f)
			if terr == nil {
				return *th
			}
			slog.Default().Error("theme: parse custom theme, falling back", "name", name, "err", terr)
		}
	}

	th, err := BuiltinTheme(name)
	if err != nil {
		slog.Default().Error("theme: unknown theme, falling back to default", "name", name, "err", err)
		th, err = BuiltinTheme("default")
		if err != nil {
			panic("tui: embedded default theme is missing or corrupt: " + err.Error())
		}
	}
	return *th
}

// BuiltinTheme loads the named bundled theme from the embedded filesystem.
func BuiltinTheme(name string) (*theme.Theme[ColorKey], error) {
	path := "themes/" + name + ".json"
	f, err := themesFS.Open(path)
	if err != nil {
		available := "run BuiltinThemeNames() for the full list"
		if names, nerr := BuiltinThemeNames(); nerr == nil {
			available = strings.Join(names, ", ")
		}
		return nil, fmt.Errorf("unknown builtin theme %q (available: %s)", name, available)
	}
	defer f.Close()
	return theme.LoadTheme[ColorKey](f)
}

func customThemeDir() string {
	return filepath.Join(config.XDGConfigDir(), "themes")
}

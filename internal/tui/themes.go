package tui

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
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

// loadThemeFromConfig resolves the theme from a ThemeConfig: custom file takes
// precedence over a builtin name. Falls back to "default" on any error.
func loadThemeFromConfig(cfg config.ThemeConfig) theme.Theme[ColorKey] {
	var th *theme.Theme[ColorKey]
	var err error

	if cfg.File != "" {
		f, ferr := os.Open(cfg.File)
		if ferr != nil {
			slog.Default().Error("theme: open custom file, falling back to default", "file", cfg.File, "err", ferr)
		} else {
			defer f.Close()
			th, err = theme.LoadTheme[ColorKey](f)
			if err != nil {
				slog.Default().Error("theme: parse custom file, falling back to default", "file", cfg.File, "err", err)
			}
		}
	}

	if th == nil {
		name := cfg.Name
		if name == "" {
			name = "default"
		}
		th, err = BuiltinTheme(name)
		if err != nil {
			slog.Default().Error("theme: unknown builtin, falling back to default", "name", name, "err", err)
			th, err = BuiltinTheme("default")
			if err != nil {
				panic("tui: embedded default theme is missing or corrupt: " + err.Error())
			}
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

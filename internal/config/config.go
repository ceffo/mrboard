// Package config provides YAML-based configuration loading for mrboard.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitLab holds credentials and settings for the GitLab API.
type GitLab struct {
	URL               string `yaml:"url"`
	Token             string `yaml:"token"`
	RequiredApprovals int    `yaml:"required_approvals"`
}

// Source describes a single source of MRs — either a group or a user.
type Source struct {
	Type     string `yaml:"type"`     // "group" or "user"
	ID       string `yaml:"id"`       // used when type == "group"
	Username string `yaml:"username"` // used when type == "user"
}

// Config is the top-level application configuration loaded from mrboard.yaml.
type Config struct {
	GitLab             GitLab   `yaml:"gitlab"`
	Sources            []Source `yaml:"sources"`
	ExcludedAuthors    []string `yaml:"excluded_authors"`
	StaleThresholdDays int      `yaml:"stale_threshold_days"` // 0 = no stale filtering
	CurrentUser        string   `yaml:"current_user"`         // enables "my view" toggle (tab)
}

// Load reads and validates the configuration from the first file found in the
// search path: $MRBOARD_CONFIG → $XDG_CONFIG_HOME/mrboard/mrboard.yaml →
// ~/.config/mrboard/mrboard.yaml → ./mrboard.yaml.
func Load() (*Config, error) {
	paths := searchPaths()

	var f *os.File
	var chosenPath string
	for _, p := range paths {
		var err error
		f, err = os.Open(filepath.Clean(p))
		if err == nil {
			chosenPath = p
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config: open %q: %w", p, err)
		}
	}
	if f == nil {
		return nil, fmt.Errorf("config: no config file found (tried: %s)", strings.Join(paths, ", "))
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", chosenPath, err)
	}

	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		cfg.GitLab.Token = token
	}

	if cfg.GitLab.RequiredApprovals == 0 {
		cfg.GitLab.RequiredApprovals = 2
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// searchPaths returns the ordered list of config file locations to try.
func searchPaths() []string {
	if v := os.Getenv("MRBOARD_CONFIG"); v != "" {
		return []string{v}
	}

	var paths []string

	xdgHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdgHome = filepath.Join(home, ".config")
		}
	}
	if xdgHome != "" {
		paths = append(paths, filepath.Join(xdgHome, "mrboard", "mrboard.yaml"))
	}

	paths = append(paths, "mrboard.yaml")
	return paths
}

func validate(cfg *Config) error {
	if cfg.GitLab.URL == "" {
		return errors.New("config: gitlab.url is required")
	}
	if cfg.GitLab.Token == "" {
		return errors.New("config: gitlab.token is required (or set $GITLAB_TOKEN)")
	}
	if len(cfg.Sources) == 0 {
		return errors.New("config: at least one sources entry is required")
	}
	return nil
}

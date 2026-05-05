// Package config provides YAML-based configuration loading for mrboard.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultPath = "mrboard.yaml"

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
	GitLab          GitLab   `yaml:"gitlab"`
	Sources         []Source `yaml:"sources"`
	ExcludedAuthors []string `yaml:"excluded_authors"`
}

// Load reads and validates the configuration from mrboard.yaml or $MRBOARD_CONFIG.
func Load() (*Config, error) {
	path := defaultPath
	if v := os.Getenv("MRBOARD_CONFIG"); v != "" {
		path = v
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // error on unrecognized keys

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
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

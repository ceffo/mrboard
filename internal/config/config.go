package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

const defaultPath = "mrboard.toml"

type GitLab struct {
	URL               string `toml:"url"`
	Token             string `toml:"token"`
	RequiredApprovals int    `toml:"required_approvals"`
}

type Source struct {
	Type     string `toml:"type"`     // "group" or "user"
	ID       string `toml:"id"`       // used when type == "group"
	Username string `toml:"username"` // used when type == "user"
}

type Config struct {
	GitLab  GitLab   `toml:"gitlab"`
	Sources []Source `toml:"sources"`
}

func Load() (*Config, error) {
	path := defaultPath
	if v := os.Getenv("MRBOARD_CONFIG"); v != "" {
		path = v
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
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
		return errors.New("config: at least one [[sources]] entry is required")
	}
	return nil
}

// Package config loads and validates mrboard configuration from a YAML file.
// It uses Viper for file loading and env-variable binding, and ozzo-validation
// for declarative validation rules.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/spf13/viper"
)

// Source describes a single source of MRs.
// IDs holds group paths (type "group") or usernames (type "user").
type Source struct {
	Type string   `mapstructure:"type"` // "group" or "user"
	IDs  []string `mapstructure:"ids"`
}

// GitLab mirrors the [gitlab] YAML section for Viper unmarshalling.
// Exported so existing call-sites (e.g. cfg.GitLab.URL) continue to compile
// during the architecture migration.
type GitLab struct {
	URL     string        `mapstructure:"url"`
	Token   string        `mapstructure:"token"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// logSection mirrors the [log] YAML section.
type logSection struct {
	Path  string `mapstructure:"path"`
	Level string `mapstructure:"level"`
}

// AppConfig is the top-level application configuration.
// Field access patterns (e.g. cfg.GitLab.URL, cfg.Sources) are intentionally
// preserved from the previous Config type so existing call-sites keep working
// while the architecture migrates.
type AppConfig struct {
	GitLab             GitLab        `mapstructure:"gitlab"`
	Sources            []Source      `mapstructure:"sources"`
	ExcludedAuthors    []string      `mapstructure:"excluded_authors"`
	CurrentUser        string        `mapstructure:"current_user"`
	Log                logSection    `mapstructure:"log"`
	LifetimeWarnAfter  time.Duration `mapstructure:"lifetime_warn_after"`
	LifetimeErrorAfter time.Duration `mapstructure:"lifetime_error_after"`
}

// Config is a backward-compatible alias for AppConfig.
// Deprecated: new code should refer to AppConfig and use the typed sub-config
// accessor methods (GitLabClientConfig, GitLabAdapterConfig, etc.).
type Config = AppConfig

// --- Typed sub-config types (used by adapters and services) -----------------

// GitLabClientConfig is the configuration consumed by pkg/gitlab.Client.
type GitLabClientConfig struct {
	URL     string
	Token   string
	Timeout time.Duration
}

// GitLabAdapterConfig is the configuration consumed by internal/adapters/gitlabadpt.
type GitLabAdapterConfig struct {
	Sources         []Source
	ExcludedAuthors []string
	CurrentUser     string
}

// MRServiceConfig is the configuration consumed by internal/domain/service/mrsvc.
type MRServiceConfig struct {
	Sources         []Source
	ExcludedAuthors []string
	CurrentUser     string
}

// LogConfig is the configuration consumed by internal/log.
type LogConfig struct {
	Path  string
	Level string
}

// --- Accessors --------------------------------------------------------------

// GitLabClientConfig extracts the configuration slice consumed by pkg/gitlab.Client.
func (c *AppConfig) GitLabClientConfig() GitLabClientConfig {
	return GitLabClientConfig{
		URL:     c.GitLab.URL,
		Token:   c.GitLab.Token,
		Timeout: c.GitLab.Timeout,
	}
}

// GitLabAdapterConfig extracts the configuration slice consumed by internal/adapters/gitlabadpt.
func (c *AppConfig) GitLabAdapterConfig() GitLabAdapterConfig {
	return GitLabAdapterConfig{
		Sources:         c.Sources,
		ExcludedAuthors: c.ExcludedAuthors,
		CurrentUser:     c.CurrentUser,
	}
}

// MRServiceConfig extracts the configuration slice consumed by internal/domain/service/mrsvc.
func (c *AppConfig) MRServiceConfig() MRServiceConfig {
	return MRServiceConfig{
		Sources:         c.Sources,
		ExcludedAuthors: c.ExcludedAuthors,
		CurrentUser:     c.CurrentUser,
	}
}

// LogConfig extracts the configuration slice consumed by internal/log.
func (c *AppConfig) LogConfig() LogConfig {
	return LogConfig{Path: c.Log.Path, Level: c.Log.Level}
}

// --- Loading ----------------------------------------------------------------

// Load reads configuration from path. When path is empty, it searches:
//
//	$XDG_CONFIG_HOME/mrboard/mrboard.yaml
//	~/.config/mrboard/mrboard.yaml
//	./mrboard.yaml
//
// GITLAB_TOKEN overrides gitlab.token if set.
func Load(path string) (*AppConfig, error) {
	v := viper.New()

	v.SetDefault("gitlab.timeout", "30s")
	v.SetDefault("log.level", "info")
	v.SetDefault("lifetime_warn_after", "72h")
	v.SetDefault("lifetime_error_after", "120h")

	// GITLAB_TOKEN env override — error only occurs on empty key name, safe to ignore.
	if err := v.BindEnv("gitlab.token", "GITLAB_TOKEN"); err != nil {
		return nil, fmt.Errorf("config: bind env: %w", err)
	}

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("mrboard")
		v.SetConfigType("yaml")
		for _, dir := range xdgSearchDirs() {
			v.AddConfigPath(dir)
		}
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// --- Validation -------------------------------------------------------------

func validate(cfg *AppConfig) error {
	if err := validateGitLab(&cfg.GitLab); err != nil {
		return err
	}
	if err := validateSources(cfg.Sources); err != nil {
		return err
	}
	return nil
}

func validateGitLab(gl *GitLab) error {
	return validation.ValidateStruct(gl,
		validation.Field(&gl.URL, validation.Required, is.URL),
		validation.Field(&gl.Token, validation.Required.Error("gitlab.token is required (or set $GITLAB_TOKEN)")),
	)
}

func validateSources(sources []Source) error {
	if err := validation.Validate(sources, validation.Required, validation.Length(1, 0)); err != nil {
		return fmt.Errorf("config: sources: %w", err)
	}
	for i, src := range sources {
		if err := validateSource(src); err != nil {
			return fmt.Errorf("config: sources[%d]: %w", i, err)
		}
	}
	return nil
}

func validateSource(src Source) error {
	return validation.ValidateStruct(&src,
		validation.Field(&src.Type, validation.Required, validation.In("group", "user")),
		validation.Field(&src.IDs, validation.Required, validation.Length(1, 0).Error("ids must contain at least one entry")),
	)
}

// --- XDG helpers ------------------------------------------------------------

// XDGConfigDir returns the mrboard-specific XDG config directory.
// Returns "" on error.
func XDGConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mrboard")
}

// XDGDataDir returns the mrboard-specific XDG data directory.
// Returns "" on error.
func XDGDataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "mrboard")
}

func xdgSearchDirs() []string {
	var dirs []string
	if dir := XDGConfigDir(); dir != "" {
		dirs = append(dirs, dir)
	}
	return dirs
}

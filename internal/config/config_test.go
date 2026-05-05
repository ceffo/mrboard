package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mrboard-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func setEnv(t *testing.T, key, val string) {
	t.Helper()
	t.Setenv(key, val)
}

func TestLoadValid(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    id: my-team
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitLab.URL != "https://gitlab.example.com" {
		t.Errorf("got URL %q", cfg.GitLab.URL)
	}
	if cfg.GitLab.RequiredApprovals != 2 {
		t.Errorf("expected default RequiredApprovals=2, got %d", cfg.GitLab.RequiredApprovals)
	}
}

func TestLoadRequiredApprovalsExplicit(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc
  required_approvals: 3

sources:
  - type: group
    id: x
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitLab.RequiredApprovals != 3 {
		t.Errorf("expected 3, got %d", cfg.GitLab.RequiredApprovals)
	}
}

func TestGitlabTokenEnvOverride(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: from-file

sources:
  - type: group
    id: x
`)
	setEnv(t, "MRBOARD_CONFIG", path)
	setEnv(t, "GITLAB_TOKEN", "from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitLab.Token != "from-env" {
		t.Errorf("expected token from env, got %q", cfg.GitLab.Token)
	}
}

func TestLoadExcludedAuthors(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

excluded_authors:
  - build-bot
  - renovate

sources:
  - type: group
    id: my-team
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.ExcludedAuthors) != 2 {
		t.Fatalf("expected 2 excluded authors, got %d", len(cfg.ExcludedAuthors))
	}
	if cfg.ExcludedAuthors[0] != "build-bot" {
		t.Errorf("expected build-bot, got %q", cfg.ExcludedAuthors[0])
	}
}

func TestValidationMissingURL(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  token: glpat-abc

sources:
  - type: group
    id: x
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestValidationMissingToken(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com

sources:
  - type: group
    id: x
`)
	setEnv(t, "MRBOARD_CONFIG", path)
	os.Unsetenv("GITLAB_TOKEN")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestValidationMissingSources(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty sources")
	}
}

func TestUnrecognizedKeyErrors(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc
  typo_key: oops

sources:
  - type: group
    id: x
`)
	setEnv(t, "MRBOARD_CONFIG", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unrecognized key")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	dir := t.TempDir()
	content := `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    id: x
`
	if err := os.WriteFile(filepath.Join(dir, "mrboard.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("MRBOARD_CONFIG")
	os.Unsetenv("GITLAB_TOKEN")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with default path: %v", err)
	}
	if cfg.GitLab.URL == "" {
		t.Error("expected URL to be loaded")
	}
}

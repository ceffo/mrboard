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

func TestLoadValid(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    ids: [my-team]
`)
	cfg, err := Load(path)
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
    ids: [x]
`)
	cfg, err := Load(path)
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
    ids: [x]
`)
	t.Setenv("GITLAB_TOKEN", "from-env")

	cfg, err := Load(path)
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
    ids: [my-team]
`)
	cfg, err := Load(path)
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
    ids: [x]
`)
	_, err := Load(path)
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
    ids: [x]
`)
	os.Unsetenv("GITLAB_TOKEN")
	_, err := Load(path)
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
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty sources")
	}
}

func TestValidationInvalidSourceType(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: invalid
    ids: [x]
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid source type")
	}
}

func TestValidationGroupSourceMissingIDs(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for group source missing ids")
	}
}

func TestValidationSourceMissingIDs(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: user
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for source missing ids")
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
    ids: [x]
`
	if err := os.WriteFile(filepath.Join(dir, "mrboard.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("GITLAB_TOKEN")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error with default path: %v", err)
	}
	if cfg.GitLab.URL == "" {
		t.Error("expected URL to be loaded")
	}
}

func TestSubConfigAccessors(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc
  timeout: 60s
  required_approvals: 3

sources:
  - type: group
    ids: [my-team]
  - type: user
    ids: [alice]

excluded_authors:
  - bot

current_user: alice
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	glClient := cfg.GitLabClientConfig()
	if glClient.URL != "https://gitlab.example.com" {
		t.Errorf("GitLabClientConfig.URL = %q", glClient.URL)
	}
	if glClient.Timeout.String() != "1m0s" {
		t.Errorf("GitLabClientConfig.Timeout = %v", glClient.Timeout)
	}

	glAdapt := cfg.GitLabAdapterConfig()
	if glAdapt.RequiredApprovals != 3 {
		t.Errorf("GitLabAdapterConfig.RequiredApprovals = %d", glAdapt.RequiredApprovals)
	}
	if len(glAdapt.Sources) != 2 {
		t.Errorf("GitLabAdapterConfig.Sources len = %d", len(glAdapt.Sources))
	}

	mrSvc := cfg.MRServiceConfig()
	if mrSvc.CurrentUser != "alice" {
		t.Errorf("MRServiceConfig.CurrentUser = %q", mrSvc.CurrentUser)
	}
	if len(mrSvc.ExcludedAuthors) != 1 {
		t.Errorf("MRServiceConfig.ExcludedAuthors len = %d", len(mrSvc.ExcludedAuthors))
	}
}

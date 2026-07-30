package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mrboard-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.example.com", cfg.GitLab.URL)
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
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.GitLab.Token)
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
	require.NoError(t, err)
	require.Len(t, cfg.ExcludedAuthors, 2)
	assert.Equal(t, "build-bot", cfg.ExcludedAuthors[0])
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
	assert.Error(t, err, "expected error for missing URL")
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
	assert.Error(t, err, "expected error for missing token")
}

func TestValidationMissingSources(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc
`)
	_, err := Load(path)
	assert.Error(t, err, "expected error for empty sources")
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
	assert.Error(t, err, "expected error for invalid source type")
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
	assert.Error(t, err, "expected error for group source missing ids")
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
	assert.Error(t, err, "expected error for source missing ids")
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mrboard.yaml"), []byte(content), 0o600))

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	require.NoError(t, os.Chdir(dir))
	os.Unsetenv("GITLAB_TOKEN")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.GitLab.URL, "expected URL to be loaded")
}

func TestLoadCommandsValid(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    ids: [my-team]

commands:
  - name: Review in tuicr
    key: d
    binary: tuicr
    args: ["{{.ProjectPath}}", "--mr", "{{.IID}}"]
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Commands, 1)
	assert.Equal(t, "Review in tuicr", cfg.Commands[0].Name)
	assert.Equal(t, "d", cfg.Commands[0].Key)
	assert.Equal(t, "tuicr", cfg.Commands[0].Binary)
	assert.Equal(t, []string{"{{.ProjectPath}}", "--mr", "{{.IID}}"}, cfg.Commands[0].Args)
}

func TestValidationDuplicateCommandKey(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    ids: [my-team]

commands:
  - name: Review in tuicr
    key: d
    binary: tuicr
  - name: Review in hunk
    key: d
    binary: hunk
`)
	_, err := Load(path)
	assert.Error(t, err, "expected error for duplicate command key")
}

func TestValidationCommandMissingName(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    ids: [my-team]

commands:
  - key: d
    binary: tuicr
`)
	_, err := Load(path)
	assert.Error(t, err, "expected error for command missing name")
}

func TestValidationCommandMissingBinaryIsWarningOnly(t *testing.T) {
	path := writeTemp(t, `
gitlab:
  url: https://gitlab.example.com
  token: glpat-abc

sources:
  - type: group
    ids: [my-team]

commands:
  - name: Review in nonexistent-tool
    key: d
    binary: mrboard-nonexistent-binary-xyz
`)
	cfg, err := Load(path)
	require.NoError(t, err, "missing binary must not fail config load")
	require.Len(t, cfg.Commands, 1)
	assert.Equal(t, "mrboard-nonexistent-binary-xyz", cfg.Commands[0].Binary,
		"command stays enabled despite missing binary")
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
	require.NoError(t, err)

	glClient := cfg.GitLabClientConfig()
	assert.Equal(t, "https://gitlab.example.com", glClient.URL, "GitLabClientConfig.URL")
	assert.Equal(t, "1m0s", glClient.Timeout.String(), "GitLabClientConfig.Timeout")

	glAdapt := cfg.GitLabAdapterConfig()
	assert.Len(t, glAdapt.Sources, 2, "GitLabAdapterConfig.Sources len")

	mrSvc := cfg.MRServiceConfig()
	assert.Equal(t, "alice", mrSvc.CurrentUser, "MRServiceConfig.CurrentUser")
	assert.Len(t, mrSvc.ExcludedAuthors, 1, "MRServiceConfig.ExcludedAuthors len")
}

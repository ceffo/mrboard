package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
)

const argvTestCommandName = "review"

func TestBuildCommandArgv(t *testing.T) {
	mr := domain.MergeRequest{
		IID:          42,
		Title:        "Fix the thing",
		Author:       editorTestApprover,
		WebURL:       "https://gitlab.example.com/group/repo/-/merge_requests/42",
		ProjectPath:  "group/repo",
		SourceBranch: "feature/fix",
		TargetBranch: "main",
	}

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{
			name: "ProjectPath",
			args: []string{"{{.ProjectPath}}"},
			want: []string{"group/repo"},
		},
		{
			name: "IID",
			args: []string{"{{.IID}}"},
			want: []string{"42"},
		},
		{
			name: "SourceBranch",
			args: []string{"{{.SourceBranch}}"},
			want: []string{"feature/fix"},
		},
		{
			name: "TargetBranch",
			args: []string{"{{.TargetBranch}}"},
			want: []string{"main"},
		},
		{
			name: "WebURL",
			args: []string{"{{.WebURL}}"},
			want: []string{"https://gitlab.example.com/group/repo/-/merge_requests/42"},
		},
		{
			name: "Title",
			args: []string{"{{.Title}}"},
			want: []string{"Fix the thing"},
		},
		{
			name: "Author",
			args: []string{"{{.Author}}"},
			want: []string{editorTestApprover},
		},
		{
			name: "multiple variables across multiple args",
			args: []string{"--mr={{.IID}}", "{{.ProjectPath}}@{{.SourceBranch}}"},
			want: []string{"--mr=42", "group/repo@feature/fix"},
		},
		{
			name: "literal arg with no template",
			args: []string{argvTestCommandName},
			want: []string{argvTestCommandName},
		},
		{
			name:    "unknown variable",
			args:    []string{"{{.NotAField}}"},
			wantErr: "resolving arg 0 template",
		},
		{
			name:    "malformed template",
			args:    []string{"{{.Title"},
			wantErr: "parsing arg 0 template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := config.Command{Name: argvTestCommandName, Binary: "hunk", Args: tt.args}

			got, err := BuildCommandArgv(mr, cmd)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

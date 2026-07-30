package tui

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
)

// commandTemplateData is the explicit, named projection of domain.MergeRequest exposed
// to configured-command argv templates (docs/adr/0004-external-command-launcher.md,
// "Template variable set"). It is deliberately not domain.MergeRequest itself: a future
// field added to that struct must never silently become a new template variable in
// users' mrboard.yaml.
type commandTemplateData struct {
	ProjectPath  string
	IID          int
	SourceBranch string
	TargetBranch string
	WebURL       string
	Title        string
	Author       string
}

func newCommandTemplateData(mr domain.MergeRequest) commandTemplateData {
	return commandTemplateData{
		ProjectPath:  mr.ProjectPath,
		IID:          mr.IID,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		WebURL:       mr.WebURL,
		Title:        mr.Title,
		Author:       mr.Author,
	}
}

// BuildCommandArgv resolves a configured command's argv template (cmd.Args) against an
// MR's template-variable projection, producing the resolved arguments to pass alongside
// cmd.Binary to exec.Command. It performs no execution itself.
func BuildCommandArgv(mr domain.MergeRequest, cmd config.Command) ([]string, error) {
	data := newCommandTemplateData(mr)

	argv := make([]string, 0, len(cmd.Args))

	for i, arg := range cmd.Args {
		tmpl, err := template.New(fmt.Sprintf("%s[%d]", cmd.Name, i)).Parse(arg)
		if err != nil {
			return nil, fmt.Errorf("command %q: parsing arg %d template %q: %w", cmd.Name, i, arg, err)
		}

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("command %q: resolving arg %d template %q: %w", cmd.Name, i, arg, err)
		}

		argv = append(argv, buf.String())
	}

	return argv, nil
}

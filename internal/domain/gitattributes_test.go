package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGeneratedRules(t *testing.T) {
	content := `# comment
*.go text eol=lf
gen/** linguist-generated
api/*.pb.go gitlab-generated=true linguist-generated=false
manual/** -gitlab-generated
vendor/ linguist-generated
"quoted name" linguist-generated
[attr]gen linguist-generated
keep.go !linguist-generated
*.snap linguist-generated=1
bad[ linguist-generated
-linguist-generated
`
	assert.Equal(t, []GeneratedRule{
		{Glob: "gen/**", Attr: AttrLinguistGenerated, State: AttrSet},
		{Glob: "api/*.pb.go", Attr: AttrGitLabGenerated, State: AttrSet},
		{Glob: "api/*.pb.go", Attr: AttrLinguistGenerated, State: AttrUnset},
		{Glob: "manual/**", Attr: AttrGitLabGenerated, State: AttrUnset},
		{Glob: "keep.go", Attr: AttrLinguistGenerated, State: AttrUnspecified},
		{Glob: "*.snap", Attr: AttrLinguistGenerated, State: AttrSet},
	}, ParseGeneratedRules(content))
}

func TestParseGeneratedRules_NoFile(t *testing.T) {
	assert.Nil(t, ParseGeneratedRules(""))
}

func TestMarkedGenerated(t *testing.T) {
	rules := ParseGeneratedRules(`gen/** linguist-generated
gen/keep.go -linguist-generated
gen/reset.go !linguist-generated
*.pb.go gitlab-generated
api/** -gitlab-generated
manual/** -gitlab-generated linguist-generated
schema/** linguist-generated
schema/** linguist-generated=false
`)
	cases := []struct {
		path string
		want bool
	}{
		{"gen/a.go", true},
		{"gen/keep.go", false},  // a later -attr overrides an earlier match
		{"gen/reset.go", false}, // !attr returns it to unspecified
		{"x/y.pb.go", true},
		{"api/v1.pb.go", false}, // -gitlab-generated overrides an earlier set
		{"manual/a.md", false},  // an unset gitlab-generated wins over linguist-generated
		{"schema/s.json", false},
		{"cmd.go", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, markedGenerated(rules, c.path), c.path)
	}
}

func TestNewSizeExclusions_GeneratedRules(t *testing.T) {
	rules := ParseGeneratedRules("gen/** linguist-generated\ngo.sum -linguist-generated\n")
	m := NewSizeExclusions(true, nil, rules).MeasureReviewSize([]FileDiffStat{
		{Path: "cmd.go", Additions: 10},
		{Path: "gen/client.go", Additions: 500},
		{Path: "go.sum", Additions: 40},
	})

	assert.Equal(t, ReviewSize{Lines: 10, Known: true}, m.Size)
	assert.Equal(t, []FileDiffStat{{Path: "gen/client.go", Additions: 500}, {Path: "go.sum", Additions: 40}},
		m.Excluded, "an unset marking never re-includes a file a built-in glob excludes")
}

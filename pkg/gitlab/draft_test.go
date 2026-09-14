package gitlab

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const strippedTitle = "add feature"

func TestStripDraftPrefix(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "Draft: prefix", title: "Draft: " + strippedTitle, want: strippedTitle},
		{name: "[Draft] prefix", title: "[Draft] " + strippedTitle, want: strippedTitle},
		{name: "(Draft) prefix", title: "(Draft) " + strippedTitle, want: strippedTitle},
		{name: "bare Draft prefix", title: "Draft " + strippedTitle, want: strippedTitle},
		{name: "WIP: prefix", title: "WIP: " + strippedTitle, want: strippedTitle},
		{name: "[WIP] prefix", title: "[WIP] " + strippedTitle, want: strippedTitle},
		{name: "bare WIP prefix", title: "WIP " + strippedTitle, want: strippedTitle},
		{name: "lowercase draft prefix", title: "draft: " + strippedTitle, want: strippedTitle},
		{name: "mixed-case draft prefix", title: "DrAfT: " + strippedTitle, want: strippedTitle},
		{name: "no marker returns title unchanged", title: strippedTitle, want: strippedTitle},
		{name: "marker word inside title is not stripped", title: "add draft support", want: "add draft support"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stripDraftPrefix(tc.title), "stripDraftPrefix(%q)", tc.title)
		})
	}
}

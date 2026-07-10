package gitlabadpt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

const testBotUser = "bot"

func TestMRDeduplicator_Deduplicate(t *testing.T) {
	mr := func(projectID, iid int, author string) domain.MergeRequest {
		return domain.MergeRequest{ProjectID: projectID, IID: iid, Author: author}
	}

	tests := []struct {
		name     string
		excluded []string
		input    []domain.MergeRequest
		want     []domain.MergeRequest
	}{
		{
			name:  "empty input",
			input: nil,
			want:  []domain.MergeRequest{},
		},
		{
			name:  "no duplicates, no exclusions",
			input: []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 2, "bob"), mr(2, 1, "charlie")},
			want:  []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 2, "bob"), mr(2, 1, "charlie")},
		},
		{
			name:  "duplicate same project+iid — first occurrence wins",
			input: []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 1, "alice")},
			want:  []domain.MergeRequest{mr(1, 1, "alice")},
		},
		{
			name:  "cross-source duplicate — first occurrence wins",
			input: []domain.MergeRequest{mr(1, 1, "alice"), mr(2, 2, "bob"), mr(1, 1, "alice")},
			want:  []domain.MergeRequest{mr(1, 1, "alice"), mr(2, 2, "bob")},
		},
		{
			name:     "excluded author dropped",
			excluded: []string{testBotUser},
			input:    []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 2, testBotUser), mr(1, 3, "charlie")},
			want:     []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 3, "charlie")},
		},
		{
			name:     "excluded author dedup — both occurrences dropped",
			excluded: []string{testBotUser},
			input:    []domain.MergeRequest{mr(1, 1, testBotUser), mr(1, 1, testBotUser)},
			want:     []domain.MergeRequest{},
		},
		{
			name:     "multiple excluded authors",
			excluded: []string{testBotUser, "ci-user"},
			input:    []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 2, testBotUser), mr(1, 3, "ci-user"), mr(1, 4, "bob")},
			want:     []domain.MergeRequest{mr(1, 1, "alice"), mr(1, 4, "bob")},
		},
		{
			name:  "same IID different projects — both kept",
			input: []domain.MergeRequest{mr(1, 1, "alice"), mr(2, 1, "alice")},
			want:  []domain.MergeRequest{mr(1, 1, "alice"), mr(2, 1, "alice")},
		},
		{
			name:  "order preserved — first occurrence determines position",
			input: []domain.MergeRequest{mr(1, 3, "charlie"), mr(1, 1, "alice"), mr(1, 2, "bob"), mr(1, 1, "alice")},
			want:  []domain.MergeRequest{mr(1, 3, "charlie"), mr(1, 1, "alice"), mr(1, 2, "bob")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := MRDeduplicator{ExcludedAuthors: tt.excluded}
			got := d.Deduplicate(tt.input)
			require.Len(t, got, len(tt.want), "got=%v want=%v", got, tt.want)
			for i := range got {
				assert.Equal(t, tt.want[i], got[i], "index %d", i)
			}
		})
	}
}

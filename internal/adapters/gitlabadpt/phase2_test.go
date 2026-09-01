package gitlabadpt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

func gqlMRWithIID(iid string) pkggitlab.GQLMergeRequest {
	mr := pkggitlab.GQLMergeRequest{}
	mr.IID = iid
	return mr
}

func TestChunkGQLMRs(t *testing.T) {
	cases := []struct {
		name      string
		count     int
		size      int
		wantSizes []int
	}{
		{name: "empty", count: 0, size: 12, wantSizes: nil},
		{name: "under one chunk", count: 5, size: 12, wantSizes: []int{5}},
		{name: "exact multiple", count: 24, size: 12, wantSizes: []int{12, 12}},
		{name: "remainder", count: 16, size: 12, wantSizes: []int{12, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mrs := make([]pkggitlab.GQLMergeRequest, tc.count)
			for i := range mrs {
				mrs[i] = gqlMRWithIID(string(rune('a' + i)))
			}

			chunks := chunkGQLMRs(mrs, tc.size)

			require.Len(t, chunks, len(tc.wantSizes), "want chunk count")
			for i, wantSize := range tc.wantSizes {
				assert.Len(t, chunks[i], wantSize, "chunk %d size", i)
			}
		})
	}
}

func TestChunkGQLMRs_PreservesOrder(t *testing.T) {
	mrs := []pkggitlab.GQLMergeRequest{gqlMRWithIID("1"), gqlMRWithIID("2"), gqlMRWithIID("3")}

	chunks := chunkGQLMRs(mrs, 2)

	require.Len(t, chunks, 2, "want 2 chunks")
	assert.Equal(t, []string{"1", "2"}, []string{chunks[0][0].IID, chunks[0][1].IID}, "first chunk order")
	assert.Equal(t, "3", chunks[1][0].IID, "second chunk order")
}

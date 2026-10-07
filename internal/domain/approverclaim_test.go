package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claim(id int64, approvers ...string) ApproverClaim {
	return ApproverClaim{ID: id, Approvers: NormalizeApprovers(approvers)}
}

func silent(id int64, approvers ...string) ApproverClaim {
	c := claim(id, approvers...)
	c.Silent = true
	return c
}

func released(id int64, approvers ...string) ApproverClaim {
	c := claim(id, approvers...)
	c.Released = true
	return c
}

func TestApproverSetHash(t *testing.T) {
	assert.Equal(t, ApproverSetHash([]string{"a", "b"}), ApproverSetHash([]string{"b", "a", "a"}))
	assert.NotEqual(t, ApproverSetHash([]string{"a", "b"}), ApproverSetHash([]string{"a"}))
	assert.NotEmpty(t, ApproverSetHash(nil), "the empty set must not collide with the empty-string sentinel")
}

func TestApproverClaim_FormatParseRoundTrip(t *testing.T) {
	for name, in := range map[string]ApproverClaim{
		"announce": {Approvers: []string{testUsername, "bob"}},
		"silent":   {Approvers: []string{testUsername}, Silent: true},
		"empty":    {},
		"released": {Approvers: []string{testUsername}, Released: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ParseApproverClaim(FormatApproverClaim(in))
			require.True(t, ok)
			assert.Equal(t, in.Silent, got.Silent)
			assert.Equal(t, in.Released, got.Released)
			assert.ElementsMatch(t, in.Approvers, got.Approvers)
		})
	}
}

func TestParseApproverClaim_FindsBlockInsideOtherText(t *testing.T) {
	body := "thanks!\n\n" + FormatApproverClaim(ApproverClaim{Approvers: []string{testUsername}}) + "\n\nlgtm"
	got, ok := ParseApproverClaim(body)
	require.True(t, ok)
	assert.Equal(t, []string{testUsername}, got.Approvers)
}

func TestParseApproverClaim_RejectsMalformed(t *testing.T) {
	block := func(version, attrs string) string {
		return "<!-- mrboard:approvers-claim " + version + " " + attrs + " -->\n<!-- /mrboard:approvers-claim -->"
	}
	const goodAttrs = "released=false silent=false approvers=a"
	for name, body := range map[string]string{
		"plain comment": "looks good to me",
		"no end tag":    "<!-- mrboard:approvers-claim v1 " + goodAttrs + " -->\nx",
		"bad version":   block("v2", goodAttrs),
		"bad username":  block("v1", "released=false silent=false approvers=a b"),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := ParseApproverClaim(body)
			assert.False(t, ok)
		})
	}
}

func TestFoldApproverClaims_EmptyLedger(t *testing.T) {
	st := FoldApproverClaims(nil)
	assert.True(t, st.Empty)
	assert.Empty(t, st.Announcers)
}

func TestFoldApproverClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims []ApproverClaim
		want   []int64
	}{
		{"first claim announces", []ApproverClaim{claim(1, "a")}, []int64{1}},
		{
			"racing duplicates collapse to the lowest id",
			[]ApproverClaim{claim(1, "a"), claim(2, "a"), claim(3, "a")},
			[]int64{1},
		},
		{"unordered input is replayed by id", []ApproverClaim{claim(3, "a"), claim(1, "a")}, []int64{1}},
		{
			"A then B then A announces every change",
			[]ApproverClaim{claim(1, "a"), claim(2, "b"), claim(3, "a")},
			[]int64{1, 2, 3},
		},
		{
			"silent baseline then change announces the change",
			[]ApproverClaim{silent(1, "a"), claim(2, "b")},
			[]int64{2},
		},
		{
			"silent baseline then same set announces nothing",
			[]ApproverClaim{silent(1, "a"), claim(2, "a")},
			nil,
		},
		{
			"release lets the next observer announce the same set again",
			[]ApproverClaim{claim(1, "a"), released(2, "a"), claim(3, "a")},
			[]int64{1, 3},
		},
		{
			"a stale release is ignored once another set was announced",
			[]ApproverClaim{claim(1, "a"), claim(2, "b"), released(3, "a"), claim(4, "b")},
			[]int64{1, 2},
		},
		{
			"empty set is a state like any other",
			[]ApproverClaim{claim(1, "a"), claim(2)},
			[]int64{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]int64, 0)
			for id := range FoldApproverClaims(tt.claims).Announcers {
				got = append(got, id)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestFoldApproverClaims_ExposesRunningState(t *testing.T) {
	st := FoldApproverClaims([]ApproverClaim{claim(1, "a"), claim(2, "a", "b")})
	assert.False(t, st.Empty)
	assert.Equal(t, []string{"a", "b"}, st.Approvers)
	assert.False(t, st.Released)

	st = FoldApproverClaims([]ApproverClaim{claim(1, "a"), released(2, "a")})
	assert.True(t, st.Released)
}

package gitlab

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMRQueries_SelectEachRulesApprovalsRequired(t *testing.T) {
	queries := map[string]string{
		"user":            gqlUserMRsQuery,
		"user thin":       gqlUserMRsThinQuery,
		"reviewer":        gqlReviewerMRsQuery,
		"reviewer thin":   gqlReviewerMRsThinQuery,
		"user merged":     gqlUserMergedMRsQuery,
		"reviewer merged": gqlReviewerMergedMRsQuery,
		"group merged":    gqlGroupMergedMRsQuery,
	}
	for name, q := range queries {
		rules := strings.Index(q, "rules {")
		require.NotEqual(t, -1, rules, "%s query selects approval rules", name)
		assert.Contains(t, q[rules:], "approvalsRequired", "%s query", name)
	}
}

func TestGQLApprovalRule_DecodesApprovalsRequired(t *testing.T) {
	var r GQLApprovalRule
	require.NoError(t, json.Unmarshal(
		[]byte(`{"name":"leads","type":"REGULAR","approvalsRequired":2,"eligibleApprovers":[{"username":"a"}]}`), &r))
	assert.Equal(t, 2, r.ApprovalsRequired)
}

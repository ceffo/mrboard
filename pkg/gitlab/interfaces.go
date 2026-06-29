package gitlab

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go"
)

// MRLister covers discovery: fetching lists of MRs from group and user sources.
type MRLister interface {
	ListGroupMRs(ctx context.Context, groupID, excludedAuthor string) ([]*gl.BasicMergeRequest, error)
	ListUserMRs(ctx context.Context, username string) ([]*gl.BasicMergeRequest, error)
	ListReviewerMRs(ctx context.Context, username string) ([]*gl.BasicMergeRequest, error)
	ListNonArchivedProjectIDs(ctx context.Context, groupID string) (map[int64]bool, error)
	IsProjectArchived(ctx context.Context, projectID int64) (bool, error)
	FetchUserMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error)
	FetchReviewerMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error)
}

// MREnricher covers enrichment: fetching full details for a single MR.
type MREnricher interface {
	GetMR(ctx context.Context, projectID, mrIID int64) (*gl.BasicMergeRequest, error)
	GetMRDiscussions(ctx context.Context, projectID, mrIID int64) ([]*gl.Discussion, error)
	GetMRApprovals(ctx context.Context, projectID, mrIID int64) (*gl.MergeRequestApprovals, error)
	GetMRApprovalRules(ctx context.Context, projectID, mrIID int64) ([]*gl.MergeRequestApprovalRule, error)
	GetMRDescription(ctx context.Context, projectID, mrIID int64) (string, error)
	GetMRDiffs(ctx context.Context, projectID, mrIID int64) ([]*gl.MergeRequestDiff, error)
	GetMRDiffRefs(ctx context.Context, projectID, mrIID int64) (baseSHA, headSHA string, err error)
	GetRawFileContent(ctx context.Context, projectID int64, path, ref string) ([]byte, error)
}

// MRWriter covers mutations: writing approval rules, reviewer lists, and fetching editable project members.
type MRWriter interface {
	GetProjectMembers(ctx context.Context, projectID int64, minAccessLevel int) ([]*gl.ProjectMember, error)
	CreateMRApprovalRule(
		ctx context.Context, projectID, mrIID int64, payload MRApprovalRulePayload,
	) (*gl.MergeRequestApprovalRule, error)
	UpdateMRApprovalRule(ctx context.Context, projectID, mrIID, ruleID int64, payload MRApprovalRulePayload) error
	// SetMRReviewers replaces the MR's reviewer set with the given user IDs.
	// An empty slice clears all reviewers.
	SetMRReviewers(ctx context.Context, projectID, mrIID int64, userIDs []int64) error
	// ListUsersByUsername looks up a GitLab user by exact username.
	// Returns nil, nil if no user is found.
	ListUsersByUsername(ctx context.Context, username string) (*gl.User, error)
	// UpdateMRDescription replaces the body of an MR with the given description.
	UpdateMRDescription(ctx context.Context, projectID, mrIID int64, description string) error
}

package mrboardcmd

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/adapters/statestore"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

const (
	autoTestProjectID = 42
	autoTestMRIID     = 7

	userAlice = "alice"
	userBob   = "bob"
)

// newTestCore builds a *core.Core wired to src and cfg, backed by a real
// on-disk state store rooted at t.TempDir() so execAuto's Load() call
// exercises the same path production does.
func newTestCore(t *testing.T, src mrsvc.MergeRequestSource, cfg *config.AppConfig) *core.Core {
	t.Helper()
	store, err := statestore.New(statestore.Config{Dir: t.TempDir()})
	require.NoError(t, err)
	return &core.Core{
		MRSource:   src,
		StateStore: store,
		Config:     cfg,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// eligibleMR returns an MR that satisfies every domain.AutoAssignCandidates
// criterion against a team roster containing userAlice and userBob.
func eligibleMR() domain.MergeRequest {
	return domain.MergeRequest{
		ProjectID: autoTestProjectID,
		IID:       autoTestMRIID,
		Author:    userAlice,
		Title:     "feat(OD-1): add widget",
		Phase:     domain.PhaseNeedsReview,
	}
}

func teamConfig() *config.AppConfig {
	return &config.AppConfig{
		AutoAssignReviewers: config.AutoAssignReviewers{Enabled: true},
		Sources:             []config.Source{{Type: "user", IDs: []string{userAlice, userBob}}},
	}
}

func TestExecAuto_Disabled_SkipsFetch(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no EXPECT() — any call fails the test
	cfg := &config.AppConfig{AutoAssignReviewers: config.AutoAssignReviewers{Enabled: false}}
	ctx := context.WithValue(context.Background(), coreKey{}, newTestCore(t, src, cfg))

	err := execAuto(ctx, autoCmdOptions{})

	require.NoError(t, err)
}

func TestExecAuto_DryRun_DoesNotWriteReviewers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().
		FetchAll(mock.Anything, mrsvc.FetchOptions{IncludeReviewerMRs: false}).
		Return([]domain.MergeRequest{eligibleMR()}, nil).Once()
	src.EXPECT().
		ResolveUsers(mock.Anything, []string{userAlice, userBob}).
		Return([]domain.User{{ID: 1, Username: userAlice}, {ID: 2, Username: userBob}}, nil).Once()
	// No SetReviewers expectation: dry run must never call it.
	ctx := context.WithValue(context.Background(), coreKey{}, newTestCore(t, src, teamConfig()))

	err := execAuto(ctx, autoCmdOptions{dryRun: true})

	require.NoError(t, err)
}

func TestExecAuto_AssignsReviewers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().
		FetchAll(mock.Anything, mrsvc.FetchOptions{IncludeReviewerMRs: false}).
		Return([]domain.MergeRequest{eligibleMR()}, nil).Once()
	src.EXPECT().
		ResolveUsers(mock.Anything, []string{userAlice, userBob}).
		Return([]domain.User{{ID: 1, Username: userAlice}, {ID: 2, Username: userBob}}, nil).Once()
	src.EXPECT().
		SetReviewers(mock.Anything, int64(autoTestProjectID), int64(autoTestMRIID), []int64{2}).
		Return(nil).Once()
	ctx := context.WithValue(context.Background(), coreKey{}, newTestCore(t, src, teamConfig()))

	err := execAuto(ctx, autoCmdOptions{dryRun: false})

	require.NoError(t, err)
}

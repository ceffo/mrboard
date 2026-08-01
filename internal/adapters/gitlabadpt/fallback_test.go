package gitlabadpt

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestGQLAttemptContext_SplitsRemainingBudget pins the core of the fallback fix:
// the phase-1 GraphQL attempt must not be allowed to consume the caller's whole
// deadline, or the REST fallback inherits an expired context.
func TestGQLAttemptContext_SplitsRemainingBudget(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), time.Minute)
	defer cancelParent()

	gqlCtx, cancel := gqlAttemptContext(parent)
	defer cancel()

	gqlDeadline, ok := gqlCtx.Deadline()
	require.True(t, ok, "a parent deadline must yield a derived deadline")
	parentDeadline, ok := parent.Deadline()
	require.True(t, ok)

	assert.True(t, gqlDeadline.Before(parentDeadline),
		"the GraphQL attempt must expire before the caller's deadline so REST has budget left")
	remaining := time.Until(gqlDeadline)
	assert.Greater(t, remaining, 20*time.Second, "the GraphQL attempt must keep a usable share")
	assert.Less(t, remaining, 40*time.Second, "the GraphQL attempt must not take the whole budget")
}

func TestGQLAttemptContext_NoDeadlinePassesThrough(t *testing.T) {
	gqlCtx, cancel := gqlAttemptContext(context.Background())
	defer cancel()

	_, ok := gqlCtx.Deadline()
	assert.False(t, ok, "with no parent deadline there is nothing to divide")
}

func TestGQLAttemptContext_ExhaustedDeadlinePassesThrough(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), -time.Second)
	defer cancelParent()

	gqlCtx, cancel := gqlAttemptContext(parent)
	defer cancel()

	assert.Error(t, gqlCtx.Err(), "an already-expired parent must stay expired")
}

// TestFetchSourceViaGQL_LiveCtxFallsBackToREST covers the case the fallback
// exists for: the GraphQL attempt burns its own sub-budget but the caller's
// deadline still has room, so REST runs.
func TestFetchSourceViaGQL_LiveCtxFallsBackToREST(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	a := &GitLabAdapter{}
	gqlFetch := func(context.Context, string) ([]pkggitlab.GQLMergeRequest, error) {
		return nil, errors.New("graphql boom")
	}
	restCalled := false
	restFallback := func(context.Context, string, *slog.Logger, time.Time) sourceResult {
		restCalled = true
		return sourceResult{}
	}

	res := a.fetchSourceViaGQL(ctx, "moncef", "user", gqlFetch, restFallback, quietLogger(), time.Now())

	assert.True(t, restCalled, "a live caller deadline must still reach the REST fallback")
	assert.Empty(t, res.errs)
}

// TestFetchSourceViaGQL_ExpiredCtxSkipsRESTFallback is the regression test for
// the observed outage behaviour: with the caller's deadline already gone, REST
// cannot issue a request, so the GraphQL error must be reported directly rather
// than logged as a fallback that silently fails at duration 0s.
func TestFetchSourceViaGQL_ExpiredCtxSkipsRESTFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()

	a := &GitLabAdapter{}
	gqlFetch := func(context.Context, string) ([]pkggitlab.GQLMergeRequest, error) {
		return nil, context.DeadlineExceeded
	}
	restCalled := false
	restFallback := func(context.Context, string, *slog.Logger, time.Time) sourceResult {
		restCalled = true
		return sourceResult{}
	}

	res := a.fetchSourceViaGQL(ctx, "moncef", "user", gqlFetch, restFallback, quietLogger(), time.Now())

	assert.False(t, restCalled, "REST must not be attempted with no budget left")
	require.Len(t, res.errs, 1, "the GraphQL failure must be surfaced as the source error")
	assert.ErrorIs(t, res.errs[0], context.DeadlineExceeded)
	assert.Contains(t, res.errs[0].Error(), `user user="moncef"`)
}

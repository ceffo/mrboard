package mrboardcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	domainmocks "github.com/ceffo/mrboard/internal/domain/mocks"
)

// runExecFetch runs execFetch against a source returning mrs and errs, and
// returns what it wrote to stdout and stderr along with its returned error.
func runExecFetch(t *testing.T, mrs []domain.MergeRequest, errs []error) (string, string, error) {
	t.Helper()
	m := newFetchMocks(t)
	m.core.Config = &config.AppConfig{}
	state := domainmocks.NewMockStateStore(t)
	state.EXPECT().Load().Return(domain.DefaultAppState(), nil)
	m.core.StateStore = state
	m.board.EXPECT().Load().Return(nil, time.Time{}, nil)
	m.board.EXPECT().Save(mock.Anything).Return(nil).Maybe()
	m.src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(mrs, errs)

	var stdout, stderr bytes.Buffer
	ctx := context.WithValue(context.Background(), coreKey{}, m.core)
	err := execFetch(ctx, fetchCmdOptions{}, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestExecFetch_SuccessPrintsMRsAndNothingOnStderr(t *testing.T) {
	stdout, stderr, err := runExecFetch(t, []domain.MergeRequest{testOpenMR}, nil)

	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Len(t, got, 1)
	assert.Empty(t, stderr)
}

func TestExecFetch_EmptySuccessPrintsEmptyArray(t *testing.T) {
	stdout, stderr, err := runExecFetch(t, nil, nil)

	require.NoError(t, err)
	assert.JSONEq(t, "[]", stdout)
	assert.Empty(t, stderr)
}

func TestExecFetch_PartialResultSucceedsWithErrorsOnStderr(t *testing.T) {
	stdout, stderr, err := runExecFetch(t, []domain.MergeRequest{testOpenMR},
		[]error{errors.New("boom one"), errors.New("boom two")})

	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Len(t, got, 1)
	assert.Equal(t, "mrboard: fetch error: boom one\nmrboard: fetch error: boom two\n", stderr)
}

func TestExecFetch_TotalFailureReturnsErrorWithEmptyStdout(t *testing.T) {
	stdout, stderr, err := runExecFetch(t, nil, []error{errors.New("boom")})

	require.ErrorIs(t, err, errFetchFailed)
	assert.Empty(t, stdout)
	assert.Equal(t, "mrboard: fetch error: boom\n", stderr)
}

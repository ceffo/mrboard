package mrboardcmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/internal/domain/service/updatesvc/mocks"
)

func TestRunSelfUpdate_UpToDate(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, "0.12.0", updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Latest: "v0.12.0"}, nil).
		Once()
	var out bytes.Buffer

	err := runSelfUpdate(context.Background(), checker, "0.12.0", &out)

	require.NoError(t, err)
	assert.Equal(t, "mrboard 0.12.0 is the latest release\n", out.String())
}

// TestRunSelfUpdate_NothingToCompare covers a build the checker cannot rank —
// a dev or git-describe binary. Reporting it as "the latest release" would be
// a lie, so it gets its own message.
func TestRunSelfUpdate_NothingToCompare(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, "dev", updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{}, nil).
		Once()
	var out bytes.Buffer

	err := runSelfUpdate(context.Background(), checker, "dev", &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "no published release to compare against")
}

func TestRunSelfUpdate_CheckerDisabled(t *testing.T) {
	var out bytes.Buffer

	err := runSelfUpdate(context.Background(), nil, "0.12.0", &out)

	require.Error(t, err)
	assert.Empty(t, out.String())
}

func TestRunSelfUpdate_CheckFails(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, mock.Anything, mock.Anything).
		Return(updatesvc.Info{}, errors.New("boom")).
		Once()
	var out bytes.Buffer

	err := runSelfUpdate(context.Background(), checker, "0.12.0", &out)

	require.Error(t, err)
	assert.Empty(t, out.String())
}

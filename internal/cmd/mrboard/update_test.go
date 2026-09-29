package mrboardcmd

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/internal/domain/service/updatesvc/mocks"
	"github.com/ceffo/mrboard/internal/selfupdate"
)

// testCurrentVersion is the running build's version in these tests; the
// matching tag is what the checker reports back when nothing newer exists.
const (
	testCurrentVersion = "0.12.0"
	testCurrentTag     = "v0.12.0"
	testNewTag         = "v1.0.0"
)

func TestRunSelfUpdate_UpToDate(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, testCurrentVersion, updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Latest: testCurrentTag}, nil).
		Once()
	var out bytes.Buffer

	err := runSelfUpdate(context.Background(), checker, testCurrentVersion, &out)

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

	err := runSelfUpdate(context.Background(), nil, testCurrentVersion, &out)

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

	err := runSelfUpdate(context.Background(), checker, testCurrentVersion, &out)

	require.Error(t, err)
	assert.Empty(t, out.String())
}

// TestCheckAndOfferUpdate_CheckerDisabled covers the pre-launch check
// (root.go's RunE, docs/adr/0010-self-update-check.md) skipping entirely when
// the feature is off — no mock is wired, so any call to it fails the test.
func TestCheckAndOfferUpdate_CheckerDisabled(t *testing.T) {
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), nil, testCurrentVersion, true, strings.NewReader(""), &out, slog.Default())

	assert.Zero(t, info)
	assert.False(t, updated)
	assert.Empty(t, out.String())
}

// TestCheckAndOfferUpdate_DevBuild_NeverChecks mirrors the version widget's
// own gating: a build with no meaningful "latest release" never makes the
// network call, even before the TUI exists to render a badge for it.
func TestCheckAndOfferUpdate_DevBuild_NeverChecks(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t) // no EXPECT() — any call fails the test
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), checker, selfupdate.DevVersion, true, strings.NewReader(""), &out, slog.Default())

	assert.Zero(t, info)
	assert.False(t, updated)
}

// TestCheckAndOfferUpdate_CheckFails_LaunchesAnyway covers a network blip: the
// check is a background nicety here, unlike `mrboard --update` where the user
// asked for the answer directly, so a failure must not block launch.
func TestCheckAndOfferUpdate_CheckFails_LaunchesAnyway(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, testCurrentVersion, updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{}, errors.New("boom")).
		Once()
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), checker, testCurrentVersion, true, strings.NewReader(""), &out, slog.Default())

	assert.Zero(t, info)
	assert.False(t, updated)
	assert.Empty(t, out.String())
}

// TestCheckAndOfferUpdate_NotAvailable_NoPrompt covers already being current:
// no update means nothing to offer, regardless of interactivity.
func TestCheckAndOfferUpdate_NotAvailable_NoPrompt(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, testCurrentVersion, updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Latest: testCurrentTag}, nil).
		Once()
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), checker, testCurrentVersion, true, strings.NewReader(""), &out, slog.Default())

	assert.Equal(t, updatesvc.Info{Latest: testCurrentTag}, info)
	assert.False(t, updated)
	assert.Empty(t, out.String())
}

// TestCheckAndOfferUpdate_Available_NonInteractive_NoPrompt covers piped
// stdin/stdout (e.g. a script or CI): the check result still seeds the TUI's
// badge, but nothing blocks waiting for an answer nobody can give.
func TestCheckAndOfferUpdate_Available_NonInteractive_NoPrompt(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, testCurrentVersion, updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Available: true, Latest: testNewTag}, nil).
		Once()
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), checker, testCurrentVersion, false, strings.NewReader(""), &out, slog.Default())

	assert.Equal(t, updatesvc.Info{Available: true, Latest: testNewTag}, info)
	assert.False(t, updated)
	assert.Empty(t, out.String())
}

// TestCheckAndOfferUpdate_Available_Interactive_Declines covers the prompt
// path up to the point where the user says no — it must not attempt the real
// upgrade run (docs/adr/0010-self-update-check.md notes that path is verified
// manually, not by automated tests, since it shells out to the real `brew`).
func TestCheckAndOfferUpdate_Available_Interactive_Declines(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, testCurrentVersion, updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Available: true, Latest: testNewTag}, nil).
		Once()
	var out bytes.Buffer

	info, updated := checkAndOfferUpdate(
		context.Background(), checker, testCurrentVersion, true, strings.NewReader("n\n"), &out, slog.Default())

	assert.Equal(t, updatesvc.Info{Available: true, Latest: testNewTag}, info)
	assert.False(t, updated)
	assert.Contains(t, out.String(), "0.12.0 → v1.0.0")
	assert.Contains(t, out.String(), "Update now?")
}

func TestConfirmYesNo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"YES with whitespace", "  YES  \n", true},
		{"lowercase n", "n\n", false},
		{"empty line", "\n", false},
		{"garbage", "sure\n", false},
		{"EOF, no input at all", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			got := confirmYesNo(strings.NewReader(tt.input), &out, "Update now?")

			assert.Equal(t, tt.want, got)
			assert.Contains(t, out.String(), "Update now?")
		})
	}
}

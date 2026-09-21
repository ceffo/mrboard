package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

// sgrPattern matches the escape sequences lipgloss wraps each styled run in.
var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// plain strips styling so assertions can match text that spans two styles —
// "6/26" is rendered as a bold "6" followed by a dim "/26".
func plain(s string) string { return sgrPattern.ReplaceAllString(s, "") }

// testHeader builds an unfiltered header showing shown MRs at the given width.
// Tests that need a filter call SetFilterState with the pre-filter total.
func testHeader(width, shown int) headerWidget {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetWidth(width)
	h.SetMRs(make([]domain.MergeRequest, shown))
	h.SetFilterState(shown, headerFilterState{})
	h.SetSort("age↓")
	h.SetSnapshotAge(time.Now(), false, "")
	return h
}

func TestHeaderWidget_AgeLabel_ZeroWrittenAt_IsEmpty(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	assert.Empty(t, h.ageLabel(), "no cached snapshot yet must render no age segment")
}

func TestHeaderWidget_AgeLabel_JustWritten_ReadsJustNow(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetSnapshotAge(time.Now(), false, "")

	assert.Equal(t, "just now", h.ageLabel())
}

func TestHeaderWidget_AgeLabel_OldSnapshot_ReadsDurationAgo(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), false, "")

	assert.Equal(t, "14m ago", h.ageLabel())
}

func TestHeaderWidget_AgeLabel_Refreshing_PrependsSpinner(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), true, "⠿")

	assert.Equal(t, "⠿ 14m ago", h.ageLabel())
}

func TestHeaderWidget_Render_IncludesAgeInStats(t *testing.T) {
	h := testHeader(120, 26)
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), false, "")

	assert.Contains(t, plain(h.render()), "14m ago")
}

func TestHeaderWidget_Render_StatsOverrideReplacesChrome(t *testing.T) {
	h := testHeader(120, 26)
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), false, "")
	h.SetStats("loading…")

	out := plain(h.render())
	assert.Contains(t, out, "loading…")
	assert.NotContains(t, out, "14m ago", "an explicit stats override (e.g. diff view) must win over the age")
}

func TestHeaderWidget_Render_Unfiltered_IsBareCountWithNoSlash(t *testing.T) {
	h := testHeader(120, 26)

	out := plain(h.render())
	assert.Contains(t, out, "26 mrs")
	assert.NotContains(t, out, "/26", "an unfiltered board must not show a shown/total ratio")
}

func TestHeaderWidget_Render_Filtered_ShowsShownOverTotal(t *testing.T) {
	h := testHeader(120, 6)
	h.SetFilterState(26, headerFilterState{Mine: true})

	assert.Contains(t, plain(h.render()), "6/26 mrs", "the slash is what signals filtering is on")
}

func TestHeaderWidget_Render_ScopeSegmentsReplaceTheUsernameSuffix(t *testing.T) {
	h := testHeader(120, 6)
	h.SetFilterState(26, headerFilterState{Mine: true, Sprint: true})

	out := plain(h.render())
	assert.Contains(t, out, "me")
	assert.Contains(t, out, "sprint")
	assert.NotContains(t, out, "@", "no individual name may reach the header")
	assert.NotContains(t, out, "[filtered]")
}

func TestHeaderWidget_Render_ReductionSegmentsCountHiddenValues(t *testing.T) {
	h := testHeader(120, 4)
	h.SetFilterState(26, headerFilterState{
		Columns: 1, Assignees: 3, Reviewers: 2, Tickets: 5,
	})

	out := plain(h.render())
	for _, want := range []string{"col×1", "asg×3", "rev×2", "tkt×5"} {
		assert.Contains(t, out, want)
	}
}

func TestHeaderWidget_Render_InactiveDimensionsAreSilent(t *testing.T) {
	h := testHeader(120, 6)
	h.SetFilterState(26, headerFilterState{Mine: true})

	out := plain(h.render())
	for _, absent := range []string{"col×", "asg×", "rev×", "tkt×", "sprint"} {
		assert.NotContains(t, out, absent, "a dimension that filters nothing must render nothing")
	}
}

func TestHeaderWidget_Render_FillsExactTerminalWidth(t *testing.T) {
	for _, width := range []int{120, 100, 80, 64, 52, 44, 36} {
		h := testHeader(width, 1)
		h.SetFilterState(26, headerFilterState{
			Mine: true, Sprint: true, Columns: 3, Assignees: 7, Reviewers: 5, Tickets: 9,
		})

		assert.Equal(t, width, lip.Width(h.render()), "header must fill exactly the terminal width at %d", width)
	}
}

func TestHeaderWidget_Render_NarrowDropsTitleBeforeFilters(t *testing.T) {
	h := testHeader(48, 1)
	h.SetFilterState(26, headerFilterState{Mine: true, Sprint: true, Assignees: 7})

	out := h.render()
	require.Equal(t, 48, lip.Width(out))
	stripped := plain(out)
	assert.NotContains(t, stripped, "mrboard", "the app name is the first thing sacrificed to width")
	assert.Contains(t, stripped, "me")
	assert.Contains(t, stripped, "sprint")
	assert.Contains(t, stripped, "asg×7")
}

func TestHeaderWidget_Render_DroppedSegmentsBecomeOverflowMarker(t *testing.T) {
	h := testHeader(36, 1)
	h.SetFilterState(26, headerFilterState{
		Mine: true, Sprint: true, Columns: 3, Assignees: 7, Reviewers: 5, Tickets: 9,
	})

	out := plain(h.render())
	assert.Contains(t, out, "1/26 mrs", "the count is never dropped")
	assert.Regexp(t, `\+\d`, out, "filters dropped for width must still be counted")
}

func TestHeaderWidget_Render_SpinnerSurvivesTheAgeItLabels(t *testing.T) {
	h := testHeader(36, 1)
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), true, "⠿")
	h.SetFilterState(26, headerFilterState{
		Mine: true, Sprint: true, Columns: 3, Assignees: 7, Reviewers: 5, Tickets: 9,
	})

	out := plain(h.render())
	assert.Contains(t, out, "⠿", "an in-flight fetch must stay visible however narrow the terminal")
	assert.NotContains(t, out, "14m ago")
}

func TestHeaderWidget_Render_BorrowedHeaderHidesTheFilterBar(t *testing.T) {
	h := testHeader(120, 6)
	h.SetFilterState(26, headerFilterState{Mine: true, Assignees: 3})
	h.SetTitle("diff !42 – some change")
	h.SetStats("loading…")

	out := plain(h.render())
	assert.Contains(t, out, "diff !42")
	assert.NotContains(t, out, "6/26", "an overlay's header must not count a board that is off screen")
	assert.NotContains(t, out, "asg×3")
}

func TestHeaderWidget_Render_ZeroWidthStillRendersEveryZone(t *testing.T) {
	h := testHeader(0, 6)
	h.SetFilterState(26, headerFilterState{Mine: true})

	out := plain(h.render())
	assert.Contains(t, out, "6/26 mrs")
	assert.Contains(t, out, "mrboard")
	assert.Equal(t, 1, strings.Count(out, "\n")+1, "the header is always a single line")
}

func TestHeaderWidget_ShownTotal_UnsetDenominatorFallsBackToShown(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetMRs(make([]domain.MergeRequest, 12))

	shown, total := h.shownTotal()
	assert.Equal(t, 12, shown)
	assert.Equal(t, 12, total, "a header wired without a total must not claim MRs are hidden")
}

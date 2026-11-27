package tui

import (
	"testing"
	"time"

	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSprintTimeLeft(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	plus := func(d time.Duration) time.Time { return now.Add(d) }
	ist := time.FixedZone("IST", 5*60*60+30*60)

	cases := []struct {
		name string
		end  time.Time
		now  time.Time
		want string
	}{
		{"three days", plus(3*24*time.Hour + 2*time.Hour), now, "3d left"},
		{"exactly one day", plus(24 * time.Hour), now, "1d left"},
		{"hours under a day", plus(5*time.Hour + 20*time.Minute), now, "5h left"},
		{"last hour", plus(40 * time.Minute), now, "ends today"},
		{"at the end", now, now, "ended"},
		{"past", plus(-time.Minute), now, "ended"},
		{"zero end", time.Time{}, now, ""},
		{
			"non-UTC end, same instant as UTC",
			plus(2*24*time.Hour + time.Hour).In(ist), now, "2d left",
		},
		{
			"non-UTC now, midnight end in another zone",
			time.Date(2026, 11, 27, 0, 0, 0, 0, ist), now.In(time.FixedZone("PST", -8*60*60)), "6h left",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sprintTimeLeft(tc.end, tc.now))
		})
	}
}

func sprintHeader(width int, filter headerFilterState, name string, end time.Time) headerWidget {
	h := testHeader(width, 6)
	h.SetFilterState(26, filter)
	h.SetSprint(name, end)
	h.now = func() time.Time { return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC) }
	return h
}

func TestHeaderWidget_Sprint_FilterOnShowsNameAndTimeLeft(t *testing.T) {
	end := time.Date(2026, 11, 29, 14, 0, 0, 0, time.UTC)
	h := sprintHeader(120, headerFilterState{Sprint: true}, "Sprint 42", end)

	assert.Contains(t, plain(h.render()), "Sprint 42 · 3d left")
}

func TestHeaderWidget_Sprint_FilterOffShowsNothing(t *testing.T) {
	end := time.Date(2026, 11, 29, 14, 0, 0, 0, time.UTC)
	h := sprintHeader(120, headerFilterState{Mine: true}, "Sprint 42", end)

	assert.NotContains(t, plain(h.render()), "Sprint 42")
}

func TestHeaderWidget_Sprint_NoSprintShowsNothing(t *testing.T) {
	h := sprintHeader(120, headerFilterState{Sprint: true}, "", time.Time{})

	out := plain(h.render())
	assert.NotContains(t, out, "left")
	assert.NotContains(t, out, headerSegSep+"ended")
}

func TestHeaderWidget_Sprint_ZeroEndShowsNameOnly(t *testing.T) {
	h := sprintHeader(120, headerFilterState{Sprint: true}, "Sprint 42", time.Time{})

	out := plain(h.render())
	assert.Contains(t, out, "Sprint 42")
	assert.NotContains(t, out, "Sprint 42 ·")
}

func TestHeaderWidget_Sprint_NarrowTruncatesNameAndKeepsWidth(t *testing.T) {
	end := time.Date(2026, 11, 29, 14, 0, 0, 0, time.UTC)
	name := "Platform Reliability Sprint 42"
	full := sprintHeader(120, headerFilterState{Mine: true, Sprint: true}, name, end)
	fullW := lip.Width(full.render())
	require.Equal(t, 120, fullW)

	// Tight enough that the full name cannot fit, loose enough for a stub.
	h := sprintHeader(72, headerFilterState{Mine: true, Sprint: true}, name, end)
	out := h.render()
	stripped := plain(out)

	assert.Equal(t, 72, lip.Width(out), "never wraps or overflows")
	assert.NotContains(t, stripped, name)
	assert.Contains(t, stripped, "…")
	assert.Contains(t, stripped, "3d left")
	assert.Contains(t, stripped, "sprint", "filter-bar pieces outrank the segment")
	assert.Contains(t, stripped, "me")
}

func TestHeaderWidget_Sprint_VeryNarrowDropsSegmentBeforeFilterPieces(t *testing.T) {
	end := time.Date(2026, 11, 29, 14, 0, 0, 0, time.UTC)
	for _, width := range []int{44, 36} {
		h := sprintHeader(width, headerFilterState{Mine: true, Sprint: true, Assignees: 2}, "Sprint 42", end)
		out := h.render()

		assert.Equal(t, width, lip.Width(out), "width %d", width)
		assert.NotContains(t, out, "\n")
		assert.NotContains(t, plain(out), "Sprint 42", "segment is the first piece to go at width %d", width)
		assert.Contains(t, plain(out), "me")
	}
}

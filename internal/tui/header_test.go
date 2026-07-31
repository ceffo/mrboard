package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetWidth(120)
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), false, "")

	assert.Contains(t, h.render(), "14m ago")
}

func TestHeaderWidget_Render_StatsOverrideHidesAge(t *testing.T) {
	h := newHeaderWidget(NewStyles(LoadThemeByName("default"), true))
	h.SetWidth(120)
	h.SetSnapshotAge(time.Now().Add(-14*time.Minute), false, "")
	h.SetStats("loading…")

	assert.NotContains(t, h.render(), "14m ago", "an explicit stats override (e.g. diff view) must win over the age")
}

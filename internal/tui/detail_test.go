package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/ceffo/mrboard/internal/domain"
)

func detailHeaderLine(mr domain.MergeRequest) string {
	d := newDetailWidget(NewStyles(LoadThemeByName("default"), true))
	d.SetMR(&mr)
	d.SetSize(80, 30)
	return ansi.Strip(d.buildLines(76)[0])
}

func TestDetail_MergedMRSaysMergedAndHidesOpenPhase(t *testing.T) {
	mr := domain.MergeRequest{
		IID: 702, Phase: domain.PhaseNeedsReview,
		MergedAt: time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
	}

	line := detailHeaderLine(mr)

	assert.Contains(t, line, "Merged 2026-03-04")
	assert.NotContains(t, line, phaseLabelReview)
}

func TestDetail_OpenMRKeepsItsPhase(t *testing.T) {
	line := detailHeaderLine(domain.MergeRequest{IID: 1, Phase: domain.PhaseNeedsReview})

	assert.Contains(t, line, phaseLabelReview)
	assert.NotContains(t, line, "Merged")
}

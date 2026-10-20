package ticketsvc_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
)

func TestSprintStartDay_IsMidnightOfTheStartsOwnCalendarDay(t *testing.T) {
	// A UTC-midnight start is the 11th on a UTC-5 machine; the sprint began on the 12th.
	orig := time.Local
	time.Local = time.FixedZone("behind", -5*3600)
	t.Cleanup(func() { time.Local = orig })
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	got := ticketsvc.Sprint{Start: start}.StartDay()

	assert.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), got)
	assert.Equal(t, time.UTC, got.Location())

	cest := time.FixedZone("cest", 2*3600)
	assert.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, cest),
		ticketsvc.Sprint{Start: time.Date(2026, 10, 12, 1, 0, 0, 0, cest)}.StartDay())
}

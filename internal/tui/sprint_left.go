package tui

import (
	"strconv"
	"time"
)

const hoursPerDay = 24

// sprintTimeLeft words the time between now and a sprint's end for the
// header: "3d left" from a day out, "5h left" under a day, "ends today" in the
// last hour, "ended" once end has passed. The span is an absolute duration, so
// the zone either time carries never moves the answer. A zero end has nothing
// to count down to and reads "".
func sprintTimeLeft(end, now time.Time) string {
	if end.IsZero() {
		return ""
	}
	left := end.Sub(now)
	switch {
	case left <= 0:
		return "ended"
	case left >= hoursPerDay*time.Hour:
		return strconv.Itoa(int(left/(hoursPerDay*time.Hour))) + "d left"
	case left >= time.Hour:
		return strconv.Itoa(int(left/time.Hour)) + "h left"
	default:
		return "ends today"
	}
}

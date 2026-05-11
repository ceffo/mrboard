package tui

// scrollViewport manages line-based scrolling. It stores only the scroll
// offset; callers supply the full line slice and visible height at render
// time so there is no stale state to synchronise between mutations.
type scrollViewport struct {
	offset int
}

func (v *scrollViewport) reset() { v.offset = 0 }

func (v *scrollViewport) scrollUp() {
	if v.offset > 0 {
		v.offset--
	}
}

// scrollDown increments unconditionally; callers must use clampedOffset.
func (v *scrollViewport) scrollDown() { v.offset++ }

// clampedOffset returns v.offset clamped to [0, total-visible].
func (v scrollViewport) clampedOffset(total, visible int) int {
	maxOff := total - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if v.offset > maxOff {
		return maxOff
	}
	return v.offset
}

// window returns the visible slice of lines.
func (v scrollViewport) window(lines []string, visible int) []string {
	offset := v.clampedOffset(len(lines), visible)
	end := offset + visible
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

func (v scrollViewport) hasAbove() bool { return v.offset > 0 }
func (v scrollViewport) hasBelow(total, visible int) bool {
	return v.clampedOffset(total, visible)+visible < total
}

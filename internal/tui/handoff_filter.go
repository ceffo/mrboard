package tui

import (
	"bytes"
	"strconv"
)

// altScreenFilter removes the "leave alternate screen" private modes
// (47, 1047, 1049) from a child's terminal output so the terminal stays on the
// alternate screen the board already owns. Other bytes, and every other mode
// (including those combined in the same sequence), pass through unchanged.
// Sequences split across Write calls are held back until they are complete.
type altScreenFilter struct {
	pending []byte
}

// maxPendingSeq bounds how much of an unterminated sequence is held back.
const maxPendingSeq = 64

var csiPrivatePrefix = []byte("\x1b[?")

func isAltScreenMode(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && (n == 47 || n == 1047 || n == 1049)
}

// filter returns in with the alternate-screen exits removed. The result may be
// shorter than in; it never aliases in.
func (f *altScreenFilter) filter(in []byte) []byte {
	data := in
	if len(f.pending) > 0 {
		data = append(f.pending, in...)
		f.pending = nil
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] != 0x1b {
			next := bytes.IndexByte(data[i:], 0x1b)
			if next < 0 {
				out = append(out, data[i:]...)
				break
			}
			out = append(out, data[i:i+next]...)
			i += next
			continue
		}
		rest := data[i:]
		switch seqState(rest) {
		case seqPartial:
			f.pending = append([]byte(nil), rest...)
			return out
		case seqOther:
			out = append(out, 0x1b)
			i++
		default:
			end := csiEnd(rest)
			out = append(out, rewriteModeSeq(rest[:end])...)
			i += end
		}
	}
	return out
}

// flush returns whatever is still held back, for use when the stream ends.
func (f *altScreenFilter) flush() []byte {
	p := f.pending
	f.pending = nil
	return p
}

type seqKind int

const (
	seqOther   seqKind = iota // not a private-mode CSI: pass the ESC through
	seqPartial                // could still become one; wait for more bytes
	seqCSI                    // a complete ESC [ ? ... <final>
)

// seqState classifies the sequence starting at b[0] == ESC.
func seqState(b []byte) seqKind {
	n := min(len(b), len(csiPrivatePrefix))
	if !bytes.Equal(b[:n], csiPrivatePrefix[:n]) {
		return seqOther
	}
	if len(b) < len(csiPrivatePrefix) {
		return seqPartial
	}
	for j := len(csiPrivatePrefix); j < len(b); j++ {
		c := b[j]
		if c >= '0' && c <= '9' || c == ';' || c == ':' {
			continue
		}
		if c >= 0x40 && c <= 0x7e {
			return seqCSI
		}
		return seqOther
	}
	if len(b) >= maxPendingSeq {
		return seqOther
	}
	return seqPartial
}

// csiEnd returns the length of the complete private-mode CSI at b[0].
func csiEnd(b []byte) int {
	for j := len(csiPrivatePrefix); j < len(b); j++ {
		if b[j] >= 0x40 && b[j] <= 0x7e {
			return j + 1
		}
	}
	return len(b)
}

// rewriteModeSeq drops alternate-screen modes from a reset (`l`) sequence and
// re-emits it with the remaining modes, or nothing when none remain.
func rewriteModeSeq(seq []byte) []byte {
	if seq[len(seq)-1] != 'l' {
		return seq
	}
	params := bytes.Split(seq[len(csiPrivatePrefix):len(seq)-1], []byte{';'})
	kept := make([][]byte, 0, len(params))
	for _, p := range params {
		if !isAltScreenMode(string(p)) {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(params) {
		return seq
	}
	if len(kept) == 0 {
		return nil
	}
	out := append([]byte(nil), csiPrivatePrefix...)
	out = append(out, bytes.Join(kept, []byte{';'})...)
	return append(out, 'l')
}

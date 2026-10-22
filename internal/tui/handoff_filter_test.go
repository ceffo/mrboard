package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAltScreenFilter(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"plain text untouched", []string{"hello\r\n"}, "hello\r\n"},
		{"1049l dropped", []string{"a\x1b[?1049lb"}, "ab"},
		{"1049h kept", []string{"\x1b[?1049hx"}, "\x1b[?1049hx"},
		{"47l and 1047l dropped", []string{"\x1b[?47l\x1b[?1047l"}, ""},
		{"combined keeps the other modes", []string{"\x1b[?25;1049l"}, "\x1b[?25l"},
		{"combined all alt dropped", []string{"\x1b[?1049;1047l"}, ""},
		{"unrelated private mode kept", []string{"\x1b[?25l\x1b[?2004h"}, "\x1b[?25l\x1b[?2004h"},
		{"other CSI kept", []string{"\x1b[31mred\x1b[0m"}, "\x1b[31mred\x1b[0m"},
		{"split after ESC", []string{"a\x1b", "[?1049lb"}, "ab"},
		{"split mid params", []string{"\x1b[?10", "49l"}, ""},
		{"split before final", []string{"\x1b[?1049", "lz"}, "z"},
		{"split non-matching flushes", []string{"\x1b[?25", "hq"}, "\x1b[?25hq"},
		{"lone ESC then text", []string{"\x1b", "x"}, "\x1bx"},
		{"OSC kept", []string{"\x1b]11;?\x07"}, "\x1b]11;?\x07"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var f altScreenFilter
			var got []byte
			for _, chunk := range tc.in {
				got = append(got, f.filter([]byte(chunk))...)
			}
			got = append(got, f.flush()...)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

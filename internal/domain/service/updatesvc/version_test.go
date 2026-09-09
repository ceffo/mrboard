package updatesvc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want updatesvc.ParsedVersion
		ok   bool
	}{
		{name: "dev build", in: "dev", ok: false},
		{name: "empty string", in: "", ok: false},
		{name: "v-prefixed", in: "v0.12.0", want: updatesvc.ParsedVersion{Minor: 12}, ok: true},
		{name: "bare", in: "0.12.0", want: updatesvc.ParsedVersion{Minor: 12}, ok: true},
		{name: "prerelease suffix", in: "v0.12.0-rc1", ok: false},
		{name: "missing patch", in: "v0.12", ok: false},
		{name: "major minor patch", in: "v1.2.3", want: updatesvc.ParsedVersion{Major: 1, Minor: 2, Patch: 3}, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := updatesvc.ParseVersion(tt.in)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name            string
		current, latest updatesvc.ParsedVersion
		want            bool
	}{
		{name: "equal", current: updatesvc.ParsedVersion{Minor: 12}, latest: updatesvc.ParsedVersion{Minor: 12}, want: false},
		{
			name:    "older",
			current: updatesvc.ParsedVersion{Minor: 12},
			latest:  updatesvc.ParsedVersion{Minor: 11},
			want:    false,
		},
		{
			name:    "newer patch",
			current: updatesvc.ParsedVersion{Minor: 12, Patch: 0},
			latest:  updatesvc.ParsedVersion{Minor: 12, Patch: 1},
			want:    true,
		},
		{
			name:    "newer minor",
			current: updatesvc.ParsedVersion{Minor: 12},
			latest:  updatesvc.ParsedVersion{Minor: 13},
			want:    true,
		},
		{
			name:    "newer major",
			current: updatesvc.ParsedVersion{Major: 0, Minor: 12},
			latest:  updatesvc.ParsedVersion{Major: 1, Minor: 0},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, updatesvc.IsNewer(tt.current, tt.latest))
		})
	}
}

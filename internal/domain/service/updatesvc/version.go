package updatesvc

import (
	"strconv"
	"strings"
)

// ParsedVersion is a parsed major.minor.patch triple.
type ParsedVersion struct {
	Major, Minor, Patch int
}

// ParseVersion parses a "v0.12.0" or "0.12.0" style version string. It
// returns ok=false for "dev", empty strings, and anything with extra
// characters after the patch number (e.g. a prerelease suffix) — this
// package only ever needs to compare goreleaser's plain v*.*.* tags, and a
// version that doesn't fit that shape must be treated as "can't safely
// compare," never as "always newer" or "always older."
func ParseVersion(s string) (ParsedVersion, bool) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return ParsedVersion{}, false
	}

	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return ParsedVersion{}, false
		}
		nums[i] = n
	}
	return ParsedVersion{Major: nums[0], Minor: nums[1], Patch: nums[2]}, true
}

// IsNewer reports whether latest is strictly newer than current. Callers
// must only pass versions that ParseVersion has already accepted.
func IsNewer(current, latest ParsedVersion) bool {
	if latest.Major != current.Major {
		return latest.Major > current.Major
	}
	if latest.Minor != current.Minor {
		return latest.Minor > current.Minor
	}
	return latest.Patch > current.Patch
}

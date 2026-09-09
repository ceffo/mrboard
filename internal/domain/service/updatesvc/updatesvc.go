// Package updatesvc owns the release-update-check service port. It is
// vendor-neutral by design: internal/adapters/githubadpt implements
// UpdateChecker against GitHub; the TUI depends only on this package.
package updatesvc

import "context"

// Info describes the result of an update check.
type Info struct {
	Available bool
	Latest    string // e.g. "v0.12.0"; empty when !Available
}

// UpdateChecker is the driven port for checking whether a newer mrboard
// release exists. Implementations own their own revalidation/cache policy —
// callers may call this on every launch without worrying about hammering the
// upstream API.
type UpdateChecker interface {
	// CheckForUpdate reports whether a release newer than currentVersion is
	// available. currentVersion is the running build's version string (e.g.
	// "0.11.0"). Implementations must never report an update available when
	// currentVersion cannot be parsed as a release version (e.g. "dev").
	CheckForUpdate(ctx context.Context, currentVersion string) (Info, error)
}

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

// CheckOptions tunes a single update check.
type CheckOptions struct {
	// Force skips any implementation-side cache and performs a live lookup.
	Force bool
}

// UpdateChecker is the driven port for checking whether a newer mrboard
// release exists. Implementations own their own revalidation/cache policy —
// callers may call this on a cadence of their choosing without worrying about
// hammering the upstream API, and reach for CheckOptions.Force when a
// specific call must see the current upstream state.
type UpdateChecker interface {
	// CheckForUpdate reports whether a release newer than currentVersion is
	// available. currentVersion is the running build's version string (e.g.
	// "0.11.0"). Implementations must never report an update available when
	// currentVersion cannot be parsed as a release version (e.g. "dev").
	CheckForUpdate(ctx context.Context, currentVersion string, opts CheckOptions) (Info, error)
}

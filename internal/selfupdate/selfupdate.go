// Package selfupdate owns how an installed mrboard binary replaces itself.
// Both the TUI's update-confirm dialog and `mrboard --update` run the same
// command from here (docs/adr/0010-self-update-check.md).
package selfupdate

import "os/exec"

// Command upgrades mrboard. It is fixed, not templated: mrboard is
// distributed as a Homebrew formula and does not detect how it was installed.
const Command = "brew update && brew upgrade ceffo/tap/mrboard"

// SuccessMessage reports a completed self-update. The TUI's quit-and-print
// path and `mrboard --update` both print this exact string so the two entry
// points can never drift apart (docs/adr/0010-self-update-check.md).
const SuccessMessage = "mrboard updated — restart mrboard to use the new version"

// DevVersion is the version string of a build not produced by goreleaser.
// Such a build has no meaningful "latest release" to compare against, so
// every update-check entry point skips it before making any network call.
const DevVersion = "dev"

// ExecCmd builds the process that runs Command. Command is a constant baked
// into the binary, not user- or config-supplied input, so unlike the external
// command launcher's argv (docs/adr/0004) there is no injection surface here;
// the shell is needed only for "&&" sequencing.
func ExecCmd() *exec.Cmd { return exec.Command("sh", "-c", Command) }

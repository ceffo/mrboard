package mrboardcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/internal/selfupdate"
)

// runSelfUpdate backs `mrboard --update`: check for a newer release and, if
// there is one, run the upgrade. The check is forced past the cache — the
// user asked for the current answer, not a remembered one.
func runSelfUpdate(ctx context.Context, checker updatesvc.UpdateChecker, version string, out io.Writer) error {
	if checker == nil {
		return errors.New("update checks are disabled (set update_check.enabled: true)")
	}

	info, err := checker.CheckForUpdate(ctx, version, updatesvc.CheckOptions{Force: true})
	if err != nil {
		return fmt.Errorf("check for update: %w", err)
	}

	switch {
	case info.Available:
		fmt.Fprintf(out, "updating mrboard %s → %s\n", version, info.Latest)
		return runUpgrade(out)
	case info.Latest == "":
		fmt.Fprintf(out, "mrboard %s: no published release to compare against\n", version)
	default:
		fmt.Fprintf(out, "mrboard %s is the latest release\n", version)
	}
	return nil
}

// runUpgrade streams the upgrade's output straight to the terminal: brew
// takes minutes and reports its own progress, which is more useful than
// mrboard buffering it and replaying it at the end. The run is not bound to
// the command context — interrupting brew partway through leaves Homebrew in
// a worse state than letting it finish (docs/adr/0010-self-update-check.md).
func runUpgrade(out io.Writer) error {
	cmd := selfupdate.ExecCmd()
	cmd.Stdout, cmd.Stderr, cmd.Stdin = out, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %q: %w", selfupdate.Command, err)
	}
	fmt.Fprintln(out, selfupdate.SuccessMessage)
	return nil
}

// checkAndOfferUpdate runs the launch-time update check (root.go's RunE,
// docs/adr/0010-self-update-check.md): before the TUI starts, check for a
// newer release and, when interactive is true, offer to install it. It
// returns the check result so the caller can seed the TUI's own badge without
// forcing a second live check moments later, and whether it already ran the
// upgrade — the caller must not launch the TUI in that case, since the
// process in memory would still be the old binary. interactive is decided by
// the caller (an isatty check on the real stdin) so this function stays
// testable without a real terminal.
//
// A disabled checker or a non-release build (selfupdate.DevVersion) skips the
// network call entirely, matching the version widget's own gating. A check
// failure is logged and swallowed rather than blocking launch: the check is a
// background nicety, not something the user asked for directly the way
// `mrboard --update` is.
func checkAndOfferUpdate(
	ctx context.Context, checker updatesvc.UpdateChecker, version string, interactive bool,
	in io.Reader, out io.Writer, logger *slog.Logger,
) (updatesvc.Info, bool) {
	if checker == nil || version == selfupdate.DevVersion {
		return updatesvc.Info{}, false
	}

	info, err := checker.CheckForUpdate(ctx, version, updatesvc.CheckOptions{Force: true})
	if err != nil {
		logger.Debug("update check failed", "err", err)
		return updatesvc.Info{}, false
	}
	if !info.Available || !interactive {
		return info, false
	}

	fmt.Fprintf(out, "mrboard update available: %s → %s\n", version, info.Latest)
	if !confirmYesNo(in, out, "Update now?") {
		return info, false
	}

	if err := runUpgrade(out); err != nil {
		fmt.Fprintf(out, "update failed: %s\n", err)
		return info, false
	}
	return info, true
}

// confirmYesNo asks a plain y/N question on the terminal. Anything other than
// "y"/"yes" (case-insensitive), including EOF, counts as no.
func confirmYesNo(in io.Reader, out io.Writer, question string) bool {
	fmt.Fprintf(out, "%s [y/N] ", question)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

package mrboardcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

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
	fmt.Fprintln(out, "done — restart mrboard to use the new version")
	return nil
}

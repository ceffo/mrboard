package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/toast"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/internal/selfupdate"
)

// updateCheckResultMsg carries the result of one release check.
type updateCheckResultMsg struct {
	info updatesvc.Info
	err  error
}

// updateCheckTickMsg fires the recurring release check.
type updateCheckTickMsg struct{}

// selfUpdateRequestedMsg is what the confirm dialog emits on yes.
type selfUpdateRequestedMsg struct{}

// selfUpdateResultMsg carries the outcome of the self-update run.
type selfUpdateResultMsg struct{ err error }

// selfUpdateSucceededMsg asks the root model to quit and print message once
// the terminal is restored: the running process is still the old binary even
// after a successful update, so mrboard cannot simply resume as if nothing
// happened (docs/adr/0010-self-update-check.md).
type selfUpdateSucceededMsg struct{ message string }

// versionWidget owns everything about the running build's version: rendering
// it into the footer, checking whether a newer release exists, enabling the
// update keybinding while one does, and running the update the user confirms
// (docs/adr/0010-self-update-check.md).
type versionWidget struct {
	styles    Styles
	version   string
	latest    string
	available bool

	checker  updatesvc.UpdateChecker
	interval time.Duration
	action   *Action // the update keybinding, enabled only while available
	baseCtx  context.Context
	logger   *slog.Logger

	// precheckedInfo, when set, is the result of a check already performed
	// before the TUI started (root.go, docs/adr/0010-self-update-check.md).
	// Init() applies it directly instead of forcing a second live check
	// moments after the first.
	precheckedInfo *updatesvc.Info
}

// setPrecheckedInfo records a check performed before the TUI started. Must be
// called before Init().
func (w *versionWidget) setPrecheckedInfo(info *updatesvc.Info) { w.precheckedInfo = info }

// newVersionWidget returns the widget for the given build version. checker is
// nil when the update check is disabled or unconfigured, and interval is the
// re-check cadence; either being absent leaves the widget as a plain version
// label. action is the update keybinding, which this widget owns the
// enablement of.
func newVersionWidget(
	ctx context.Context,
	styles Styles,
	version string,
	checker updatesvc.UpdateChecker,
	interval time.Duration,
	action *Action,
	logger *slog.Logger,
) *versionWidget {
	action.SetEnabled(false)
	return &versionWidget{
		styles:   styles,
		version:  version,
		checker:  checker,
		interval: interval,
		action:   action,
		baseCtx:  ctx,
		logger:   logger,
	}
}

// SetStyles updates the widget's style set.
func (w *versionWidget) SetStyles(s Styles) { w.styles = s }

// Init applies a precheck done before the TUI started if there is one,
// otherwise forces a live lookup itself: a cache entry written by a previous
// run says what was true then, and the user opening mrboard is exactly when a
// stale badge (or a missing one) is most visible. Either way it starts the
// recurring re-check.
func (w *versionWidget) Init() tea.Cmd {
	if w.precheckedInfo != nil {
		w.applyCheckResult(updateCheckResultMsg{info: *w.precheckedInfo})
		return w.tickCmd()
	}
	return tea.Batch(w.checkCmd(updatesvc.CheckOptions{Force: true}), w.tickCmd())
}

func (w *versionWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	switch msg := msg.(type) {
	case updateCheckTickMsg:
		return w, tea.Batch(w.checkCmd(updatesvc.CheckOptions{}), w.tickCmd())
	case updateCheckResultMsg:
		w.applyCheckResult(msg)
		return w, nil
	case selfUpdateRequestedMsg:
		return w, w.selfUpdateCmd()
	case selfUpdateResultMsg:
		return w, w.selfUpdateResultCmd(msg)
	}
	return w, nil
}

func (w *versionWidget) View() tea.View { return tea.NewView(w.render()) }

// render returns the footer's right-hand segment. While an update is
// available the badge carries its own key hint: the update action is
// PriorityModal, so it never appears in the footer's binding list, and the
// version segment is the one part of the footer narrow terminals never drop.
func (w *versionWidget) render() string {
	out := w.styles.FooterVersion.Render(w.version)
	if !w.available {
		return out
	}
	return out + " " + w.styles.FooterUpdateBadge.Render("↑") + " " +
		w.styles.FooterKey.Render(w.action.Help().Key) + " " +
		w.styles.Footer.Render(w.action.Help().Desc)
}

// confirmDialog builds the yes/no dialog offered when the user presses the
// update key.
func (w *versionWidget) confirmDialog(keys ConfirmKeyMap) *confirmWidget {
	body := fmt.Sprintf("update available: %s → %s\n\nrun `%s`?", w.version, w.latest, selfupdate.Command)
	return newConfirmWidget("Update mrboard", body, w.styles, keys, func() tea.Msg {
		return selfUpdateRequestedMsg{}
	})
}

// checkCmd asks the checker whether a newer release exists, or nil when there
// is nothing to check.
func (w *versionWidget) checkCmd(opts updatesvc.CheckOptions) tea.Cmd {
	if !w.checks() {
		return nil
	}
	checker, version, base := w.checker, w.version, w.baseCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, fetchTimeout)
		defer cancel()
		info, err := checker.CheckForUpdate(ctx, version, opts)
		return updateCheckResultMsg{info: info, err: err}
	}
}

// tickCmd schedules the next recurring check. The cadence is the checker's own
// cache TTL: a shorter period would only ever be answered from that cache, and
// a longer one would leave the cache expired between checks.
func (w *versionWidget) tickCmd() tea.Cmd {
	if !w.checks() || w.interval <= 0 {
		return nil
	}
	return tea.Tick(w.interval, func(time.Time) tea.Msg { return updateCheckTickMsg{} })
}

func (w *versionWidget) checks() bool {
	return w.checker != nil && w.version != selfupdate.DevVersion
}

// applyCheckResult records the outcome of a check. A failure is logged at
// Debug and not surfaced: the check is a background nicety, not an action the
// user took, so it gets no toast.
func (w *versionWidget) applyCheckResult(msg updateCheckResultMsg) {
	if msg.err != nil {
		w.logger.Debug("tui: update check failed", "err", msg.err)
		return
	}
	w.available = msg.info.Available
	w.latest = msg.info.Latest
	w.action.SetEnabled(msg.info.Available)
}

// selfUpdateCmd suspends mrboard and runs the fixed update command via
// tea.ExecProcess, mirroring execCommandCmd's shape.
func (w *versionWidget) selfUpdateCmd() tea.Cmd {
	w.logger.Info("tui: running self-update", "from", w.version, "to", w.latest)
	return tea.ExecProcess(selfupdate.ExecCmd(), func(err error) tea.Msg {
		return selfUpdateResultMsg{err: err}
	})
}

// selfUpdateResultCmd reports the run's outcome. A failure toasts and leaves
// the TUI running so the user can retry or dismiss it. Success instead quits
// the program: the process in memory is still the old binary even after
// `brew upgrade` finishes on disk, so resuming the redrawn board would wrongly
// imply the running session is now current. selfUpdateSucceededMsg carries the
// message the root model prints once the terminal is restored.
func (w *versionWidget) selfUpdateResultCmd(msg selfUpdateResultMsg) tea.Cmd {
	if msg.err != nil {
		w.logger.Error("tui: self-update failed", "err", msg.err)
		return toastCmd(toast.ErrorAlert, "update failed: "+msg.err.Error())
	}
	w.logger.Info("tui: self-update finished")
	return func() tea.Msg { return selfUpdateSucceededMsg{message: selfupdate.SuccessMessage} }
}

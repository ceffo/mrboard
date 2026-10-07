package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/toast"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// announceTimeout bounds one claim-and-deliver round: ledger reads and writes,
// the settle pause between them, and the webhook post.
const announceTimeout = 45 * time.Second

// announceTracker remembers, per MR, the approver set this session has already
// handed to the ledger, so a refresh costs GitLab calls only for MRs whose
// approvers changed. It is session-local: the ledger, not this map, is what
// stops other instances announcing the same change.
type announceTracker struct {
	handled map[domain.MRKey][]string
	// pending holds the hash of a changed set seen once and not yet acted on.
	// A change must survive two refreshes before it is claimed, because
	// GitLab's approval-rule read can briefly return the set from before a
	// write, and claiming that would announce a revert.
	pending map[domain.MRKey]string
}

func newAnnounceTracker() announceTracker {
	return announceTracker{
		handled: make(map[domain.MRKey][]string),
		pending: make(map[domain.MRKey]string),
	}
}

// approverAnnounceResultMsg carries the outcome of one claim-and-deliver round.
type approverAnnounceResultMsg struct {
	MR        domain.MergeRequest
	Announced bool
	// Previous is the handled set to restore on failure; HadPrevious is false
	// when the MR had none, in which case the entry is dropped instead.
	Previous    []string
	HadPrevious bool
	Err         error
}

// WithApproverAnnouncements enables announcing approver changes discovered on
// any MR, not only those made through this session's editor. claims is the
// shared ledger that keeps every running instance from announcing the same
// change; newMRWindow is how recent an MR must be for its first approver set to
// count as a change. A nil claims leaves the feature off.
func (m Model) WithApproverAnnouncements(claims mrsvc.ApproverClaims, newMRWindow time.Duration) Model {
	m.approverClaims = claims
	m.newMRWindow = newMRWindow
	return m
}

func (m Model) announcingApprovers() bool {
	return m.approverClaims != nil && m.notifier != nil
}

// makeApproverAnnounceCmds returns one claim-and-deliver command per MR whose
// approver set changed since this session last handled it.
func (m *Model) makeApproverAnnounceCmds() tea.Cmd {
	if !m.announcingApprovers() {
		return nil
	}
	now := time.Now()
	var cmds []tea.Cmd
	for _, mr := range m.allMRs {
		key := mr.Key()
		if _, unconfirmed := m.dirty[key]; unconfirmed {
			continue
		}
		set := domain.NormalizeApprovers(mr.Approvers)
		prev, seen := m.announce.handled[key]
		var prior []string
		switch {
		case !seen && len(set) == 0:
			m.announce.handled[key] = set
			continue
		case !seen:
			prior = mrsvc.DiscoveryPrior(mr, now, m.newMRWindow)
		case domain.ApproverSetHash(prev) == domain.ApproverSetHash(set):
			delete(m.announce.pending, key)
			continue
		case m.announce.pending[key] != domain.ApproverSetHash(set):
			m.announce.pending[key] = domain.ApproverSetHash(set)
			continue
		default:
			prior = prev
		}
		delete(m.announce.pending, key)
		m.announce.handled[key] = set
		cmds = append(cmds, m.approverAnnounceCmd(mr, mrsvc.ClaimRequest{
			ProjectID: int64(mr.ProjectID),
			MRIID:     int64(mr.IID),
			Approvers: set,
			Prior:     prior,
		}, prev, seen))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) approverAnnounceCmd(
	mr domain.MergeRequest, req mrsvc.ClaimRequest, previous []string, hadPrevious bool,
) tea.Cmd {
	claims, notifier, base := m.approverClaims, m.notifier, m.baseCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, announceTimeout)
		defer cancel()
		announced, err := mrsvc.AnnounceApproverChange(ctx, claims, notifier, mr, req)
		return approverAnnounceResultMsg{
			MR: mr, Announced: announced, Previous: previous, HadPrevious: hadPrevious, Err: err,
		}
	}
}

// handleNotificationResult routes the outcome of either kind of Teams delivery:
// the manual notify key or an approver-change announcement.
func (m Model) handleNotificationResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if announce, ok := msg.(approverAnnounceResultMsg); ok {
		return m.handleApproverAnnounceResult(announce)
	}
	notify, _ := msg.(NotifyResultMsg)
	return m.handleNotifyResult(notify)
}

func (m Model) handleApproverAnnounceResult(msg approverAnnounceResultMsg) (tea.Model, tea.Cmd) {
	key := msg.MR.Key()
	if msg.Err != nil {
		if msg.HadPrevious {
			m.announce.handled[key] = msg.Previous
		} else {
			delete(m.announce.handled, key)
		}
		m.logger.Warn("tui: approver announcement failed", "mr_iid", msg.MR.IID, "err", msg.Err)
		return m, m.toast(toast.ErrorAlert, "Approver announcement failed")
	}
	if msg.Announced {
		m.logger.Info("tui: approver change announced", "mr_iid", msg.MR.IID)
		return m, m.toast(toast.InfoAlert, "Teams notified ✓")
	}
	return m, nil
}

// announceEditedApprovers claims and delivers an approver change this session
// just wrote. The written set is authoritative, so the claim does not wait for
// a refresh to confirm it, and the session records it as handled so the
// refresh that follows does not announce it again.
func (m *Model) announceEditedApprovers(mr domain.MergeRequest, before []string) tea.Cmd {
	var approvers []string
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			approvers = append(approvers, r.Username)
		}
	}
	set := domain.NormalizeApprovers(approvers)
	mr.Approvers = set
	key := mr.Key()
	prev, seen := m.announce.handled[key]
	m.announce.handled[key] = set
	delete(m.announce.pending, key)
	return m.approverAnnounceCmd(mr, mrsvc.ClaimRequest{
		ProjectID:     int64(mr.ProjectID),
		MRIID:         int64(mr.IID),
		Approvers:     set,
		Prior:         domain.NormalizeApprovers(before),
		Authoritative: true,
	}, prev, seen)
}

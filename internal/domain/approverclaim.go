package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ApproverClaim is one entry of an MR's approver-announcement ledger: an
// append-only log in which every mrboard instance records the approver set it
// is about to announce. The ledger's total order, assigned by the store that
// holds it, is what lets independent instances agree on which of them
// announces a given change.
type ApproverClaim struct {
	// ID is the entry's position in the ledger, assigned by the store; 0 until written.
	ID int64
	// Approvers is the sorted, deduplicated approver set the entry records.
	Approvers []string
	// Silent entries record a set without announcing it. Ledgers written by
	// mrboard v0.22.0 and v0.22.1 carry them as baselines; later versions only
	// read them.
	Silent bool
	// Released withdraws the announcement of the entry that recorded Approvers:
	// an instance that won an announcement but failed to deliver it releases
	// it, so the next instance to observe the same set announces it again.
	// A release takes effect only while that set is still the ledger's
	// running state; once another entry has moved on, it is ignored.
	Released bool
	// Legacy marks an entry written before an approver meant a required
	// approver (docs/adr/0016): its set may hold members of optional rules.
	// Entries are written in the current format; Legacy ones are only read.
	Legacy bool
	// Source is ApproverChangedByMrboard on an entry recording a set an
	// mrboard approver editor wrote. It is carried by a line inside the entry's
	// block, which readers that predate it skip, so they fold the entry as any
	// other.
	Source ApproverChangeSource
}

const (
	claimTag    = "mrboard:approvers-claim"
	claimMarker = "<!-- " + claimTag + " v2"
	claimEnd    = "<!-- /" + claimTag + " -->"
	sourceTag   = "mrboard:approvers-source"
	editorLine  = "<!-- " + sourceTag + " editor -->"
	hashLen     = 16
)

// claimPattern matches one claim block anywhere in a note body, so text a
// person adds around it does not hide the claim. v1 is the legacy format, v2
// the current one; mrboard versions that read only v1 do not see v2 entries.
var claimPattern = regexp.MustCompile(
	`(?s)<!-- ` + claimTag + ` (v1|v2) released=(true|false) silent=(true|false) approvers=([A-Za-z0-9._,-]*) -->` +
		`.*?<!-- /` + claimTag + ` -->`)

// sourcePattern matches the source line inside a claim block.
var sourcePattern = regexp.MustCompile(`<!-- ` + sourceTag + ` ([a-z]+) -->`)

// NormalizeApprovers returns usernames sorted and deduplicated.
func NormalizeApprovers(usernames []string) []string {
	out := slices.Clone(usernames)
	slices.Sort(out)
	return slices.Compact(out)
}

// ApproverSetHash identifies an approver set independent of order.
func ApproverSetHash(usernames []string) string {
	sum := sha256.Sum256([]byte(strings.Join(NormalizeApprovers(usernames), ",")))
	return hex.EncodeToString(sum[:])[:hashLen]
}

// FormatApproverClaim renders c as a ledger entry body. The block is machine
// readable and self-delimiting; the line inside it is for people reading the MR.
func FormatApproverClaim(c ApproverClaim) string {
	approvers := strings.Join(NormalizeApprovers(c.Approvers), ",")
	text := "mrboard announced approvers: " + approvers
	switch {
	case c.Released:
		text = "mrboard released its approver announcement: " + approvers
	case c.Silent:
		text = "mrboard recorded approvers: " + approvers
	}
	if c.Source == ApproverChangedByMrboard {
		text += "\n" + editorLine
	}
	return fmt.Sprintf("%s released=%t silent=%t approvers=%s -->\n%s\n%s",
		claimMarker, c.Released, c.Silent, approvers, text, claimEnd)
}

// ParseApproverClaim extracts the claim from a note body. ok is false when the
// body carries no well-formed claim block.
func ParseApproverClaim(body string) (c ApproverClaim, ok bool) {
	m := claimPattern.FindStringSubmatch(body)
	if m == nil {
		return ApproverClaim{}, false
	}
	c.Legacy = m[1] == "v1"
	c.Released = m[2] == "true"
	c.Silent = m[3] == "true"
	if m[4] != "" {
		c.Approvers = NormalizeApprovers(strings.Split(m[4], ","))
	}
	if src := sourcePattern.FindStringSubmatch(m[0]); src != nil && src[1] == "editor" {
		c.Source = ApproverChangedByMrboard
	}
	return c, true
}

// ClaimLedgerState is the outcome of folding a ledger.
type ClaimLedgerState struct {
	// Empty is true when the ledger has no entries.
	Empty bool
	// Released is true when the latest entry withdrew an announcement.
	Released bool
	// Approvers is the set the latest entry recorded.
	Approvers []string
	// Legacy is true when the latest entry is a Legacy one.
	Legacy bool
	// Announcers holds the IDs of the entries that own an announcement.
	Announcers map[int64]bool
	// Before holds, per announcing entry, the set its announcement replaces.
	Before map[int64]ClaimBefore
	// Sources holds, per announcing entry, who changed the set it announces;
	// an entry absent from it announces a change made elsewhere.
	Sources map[int64]ApproverChangeSource
}

// Source returns who changed the set the announcing entry id announces.
func (s ClaimLedgerState) Source(id int64) ApproverChangeSource {
	return s.Sources[id]
}

// ClaimBefore is the set an announcing entry replaces: the running set before
// it, or, when that set's announcement was released, the set before that one,
// since a released set was never delivered.
type ClaimBefore struct {
	Approvers []string
	// Known is false when no entry precedes the announcing one, or when the
	// preceding entry is Legacy: its set may hold members of optional rules,
	// so comparing against it would report the meaning change as a change.
	Known bool
}

// Replaced returns the set the announcing entry id replaces: the folded one
// when it is Known, else prior, the set the claimer assumed before the change.
func (s ClaimLedgerState) Replaced(id int64, prior []string) []string {
	if before := s.Before[id]; before.Known {
		return before.Approvers
	}
	return prior
}

// FoldApproverClaims replays a ledger in ID order. An entry owns an
// announcement exactly when it changes the running state and is not Silent;
// later entries repeating the running state are redundant, which is what
// collapses N instances racing to announce one change into a single winner.
// Replaying is deterministic, so every instance reading the same ledger agrees.
//
// An announcing entry's source is mrboard's editor when any entry of its run
// says so. A run is every entry from one that records a set different from the
// one before it up to the next such entry: the redundant entries of instances
// that lost the race, releases of that set, and the entry re-announcing it
// after a release. Whichever instance wins, and whether or not the editor's
// own entry delivered, the set the editor wrote is announced as its change.
func FoldApproverClaims(claims []ApproverClaim) ClaimLedgerState {
	ordered := slices.Clone(claims)
	slices.SortFunc(ordered, func(a, b ApproverClaim) int { return int(a.ID - b.ID) })

	st := ClaimLedgerState{
		Empty: true, Announcers: make(map[int64]bool), Before: make(map[int64]ClaimBefore),
		Sources: make(map[int64]ApproverChangeSource),
	}
	var running string
	var before ClaimBefore
	var run approverRun
	for _, c := range ordered {
		hash := ApproverSetHash(c.Approvers)
		if !c.Released && hash != run.hash {
			run.close(st.Sources)
			run = approverRun{hash: hash}
		}
		if hash == run.hash && c.Source == ApproverChangedByMrboard {
			run.byMrboard = true
		}
		if c.Released {
			if !st.Empty && hash == running {
				running = ""
				st.Released = true
			}
			continue
		}
		if st.Empty || hash != running {
			if !st.Released {
				before = ClaimBefore{Approvers: st.Approvers, Known: !st.Empty && !st.Legacy}
			}
			if !c.Silent {
				st.Announcers[c.ID] = true
				st.Before[c.ID] = before
				run.announcers = append(run.announcers, c.ID)
			}
		}
		running = hash
		st.Empty = false
		st.Released = false
		st.Approvers = c.Approvers
		st.Legacy = c.Legacy
	}
	run.close(st.Sources)
	return st
}

// approverRun collects the entries of one run while a ledger is folded.
type approverRun struct {
	hash       string
	announcers []int64
	byMrboard  bool
}

// close records the run's source for each of its announcing entries.
func (r approverRun) close(sources map[int64]ApproverChangeSource) {
	if !r.byMrboard {
		return
	}
	for _, id := range r.announcers {
		sources[id] = ApproverChangedByMrboard
	}
}

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
	// Silent entries record a set without announcing it. They baseline an MR
	// whose earlier history mrboard never saw.
	Silent bool
	// Released withdraws the announcement of the entry that recorded Approvers:
	// an instance that won an announcement but failed to deliver it releases
	// it, so the next instance to observe the same set announces it again.
	// A release takes effect only while that set is still the ledger's
	// running state; once another entry has moved on, it is ignored.
	Released bool
}

const (
	claimTag    = "mrboard:approvers-claim"
	claimMarker = "<!-- " + claimTag + " v1"
	claimEnd    = "<!-- /" + claimTag + " -->"
	hashLen     = 16
)

// claimPattern matches one claim block anywhere in a note body, so text a
// person adds around it does not hide the claim.
var claimPattern = regexp.MustCompile(
	`(?s)<!-- ` + claimTag + ` v1 released=(true|false) silent=(true|false) approvers=([A-Za-z0-9._,-]*) -->` +
		`.*?<!-- /` + claimTag + ` -->`)

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
	c.Released = m[1] == "true"
	c.Silent = m[2] == "true"
	if m[3] != "" {
		c.Approvers = NormalizeApprovers(strings.Split(m[3], ","))
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
	// Announcers holds the IDs of the entries that own an announcement.
	Announcers map[int64]bool
}

// FoldApproverClaims replays a ledger in ID order. An entry owns an
// announcement exactly when it changes the running state and is not Silent;
// later entries repeating the running state are redundant, which is what
// collapses N instances racing to announce one change into a single winner.
// Replaying is deterministic, so every instance reading the same ledger agrees.
func FoldApproverClaims(claims []ApproverClaim) ClaimLedgerState {
	ordered := slices.Clone(claims)
	slices.SortFunc(ordered, func(a, b ApproverClaim) int { return int(a.ID - b.ID) })

	st := ClaimLedgerState{Empty: true, Announcers: make(map[int64]bool)}
	var running string
	for _, c := range ordered {
		hash := ApproverSetHash(c.Approvers)
		if c.Released {
			if !st.Empty && hash == running {
				running = ""
				st.Released = true
			}
			continue
		}
		if (st.Empty || hash != running) && !c.Silent {
			st.Announcers[c.ID] = true
		}
		running = hash
		st.Empty = false
		st.Released = false
		st.Approvers = c.Approvers
	}
	return st
}

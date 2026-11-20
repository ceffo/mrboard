package domain

import "slices"

// ApproverChangeSource says who changed an MR's required approvers.
type ApproverChangeSource int

const (
	// ApproverChangedElsewhere is a change mrboard found on a refresh: a
	// repository rule, or a rule edited by hand.
	ApproverChangedElsewhere ApproverChangeSource = iota
	// ApproverChangedByMrboard is a change an mrboard approver editor wrote.
	ApproverChangedByMrboard
)

// ApproverChange is an announced change of an MR's required approvers
// (docs/adr/0016): who became one, who stopped being one, and who changed the
// rule.
type ApproverChange struct {
	// Gained holds the sorted usernames that became required approvers.
	Gained []string
	// Lost holds the sorted usernames that stopped being required approvers.
	Lost   []string
	Source ApproverChangeSource
}

// NewApproverChange compares two required-approver sets.
func NewApproverChange(before, after []string, source ApproverChangeSource) ApproverChange {
	b, a := NormalizeApprovers(before), NormalizeApprovers(after)
	c := ApproverChange{Source: source}
	for _, u := range a {
		if !slices.Contains(b, u) {
			c.Gained = append(c.Gained, u)
		}
	}
	for _, u := range b {
		if !slices.Contains(a, u) {
			c.Lost = append(c.Lost, u)
		}
	}
	return c
}

// Empty reports a change in which no one gained or lost the status.
func (c ApproverChange) Empty() bool { return len(c.Gained) == 0 && len(c.Lost) == 0 }

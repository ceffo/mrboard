package domain

import "slices"

// ManagedApproverRuleName is the name of the one approval rule mrboard writes.
const ManagedApproverRuleName = "approvers_mrboard"

// ApproverRule is one approval rule on an MR and the usernames eligible to
// approve under it.
type ApproverRule struct {
	Name      string
	Approvers []string // sorted
	// Managed marks the rule mrboard writes. mrboard never changes any other
	// rule: their approvers are locked as far as mrboard is concerned.
	Managed bool
}

// ManagedApprovers returns the usernames on mr's managed rule, sorted; nil when
// the MR has none.
func (mr MergeRequest) ManagedApprovers() []string {
	for _, r := range mr.ApproverRules {
		if r.Managed {
			return NormalizeApprovers(r.Approvers)
		}
	}
	return nil
}

// LockedApprovers maps each username an unmanaged rule makes an approver to the
// names of those rules, sorted. These approvers stay approvers whatever mrboard
// writes.
func (mr MergeRequest) LockedApprovers() map[string][]string {
	locked := make(map[string][]string)
	for _, r := range mr.ApproverRules {
		if r.Managed {
			continue
		}
		for _, u := range r.Approvers {
			if !slices.Contains(locked[u], r.Name) {
				locked[u] = append(locked[u], r.Name)
			}
		}
	}
	for _, names := range locked {
		slices.Sort(names)
	}
	return locked
}

// SetManagedApprovers makes usernames the managed rule's approvers, adding the
// managed rule when mr has none, and recomputes Approvers and every reviewer's
// IsApprover flag from the resulting rules.
func (mr *MergeRequest) SetManagedApprovers(usernames []string) {
	set := NormalizeApprovers(usernames)
	found := false
	rules := make([]ApproverRule, 0, len(mr.ApproverRules)+1)
	for _, r := range mr.ApproverRules {
		if r.Managed && !found {
			found = true
			r.Approvers = set
		}
		rules = append(rules, r)
	}
	if !found {
		rules = append(rules, ApproverRule{Name: ManagedApproverRuleName, Approvers: set, Managed: true})
	}
	mr.ApproverRules = rules

	union := make(map[string]bool)
	for _, r := range rules {
		for _, u := range r.Approvers {
			union[u] = true
		}
	}
	all := make([]string, 0, len(union))
	for u := range union {
		all = append(all, u)
	}
	mr.Approvers = NormalizeApprovers(all)
	if len(mr.Approvers) == 0 {
		mr.Approvers = nil
	}
	for i := range mr.Reviewers {
		mr.Reviewers[i].IsApprover = union[mr.Reviewers[i].Username]
	}
}

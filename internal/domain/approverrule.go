package domain

import "slices"

// ManagedApproverRuleName is the name of the one approval rule mrboard writes.
const ManagedApproverRuleName = "approvers_mrboard"

// ApproverRule is one approval rule on an MR and the usernames eligible to
// approve under it.
type ApproverRule struct {
	Name      string
	Approvers []string // sorted
	// UserIDs maps each approver to their user ID when the read that produced
	// the rule carries IDs; nil otherwise.
	UserIDs map[string]int64
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

// ManagedApproverIDs maps the managed rule's members to their user IDs, as far
// as the read that produced mr carried them; nil when it carried none.
func (mr MergeRequest) ManagedApproverIDs() map[string]int64 {
	for _, r := range mr.ApproverRules {
		if r.Managed {
			return r.UserIDs
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
			r.UserIDs = idsOf(r.UserIDs, set)
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

// idsOf returns the entries of ids for usernames; nil when none remain.
func idsOf(ids map[string]int64, usernames []string) map[string]int64 {
	var out map[string]int64
	for _, u := range usernames {
		if id, ok := ids[u]; ok {
			if out == nil {
				out = make(map[string]int64, len(usernames))
			}
			out[u] = id
		}
	}
	return out
}

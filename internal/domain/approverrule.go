package domain

import "slices"

// ManagedApproverRuleName is the name of the one approval rule mrboard writes.
const ManagedApproverRuleName = "approvers_mrboard"

// ApproverRule is one approval rule on an MR and the usernames eligible to
// approve under it.
type ApproverRule struct {
	Name      string
	Approvers []string // sorted
	// ApprovalsRequired is how many approvals the rule needs.
	ApprovalsRequired int
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

// LockedApprovers maps each username an unmanaged rule makes a required
// approver to the names of those rules, sorted. These approvers stay approvers
// whatever mrboard writes. An unmanaged rule that does not require every member
// locks no one: its members are not approvers.
func (mr MergeRequest) LockedApprovers() map[string][]string {
	locked := make(map[string][]string)
	for _, r := range mr.ApproverRules {
		if r.Managed || !r.RequiresEveryMember() {
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

// RequiresEveryMember reports whether every approver of r must approve: the
// rule needs at least one approval and as many as it has approvers. Only such a
// rule makes its members required approvers; a rule needing fewer approvals
// than it has members, none, or more than it has members makes no one required.
func (r ApproverRule) RequiresEveryMember() bool {
	return r.ApprovalsRequired > 0 && r.ApprovalsRequired == len(r.Approvers)
}

// RequiredApprovers returns the members of every rule that requires every
// member, sorted and deduplicated; nil when there are none.
func RequiredApprovers(rules []ApproverRule) []string {
	var all []string
	for _, r := range rules {
		if r.RequiresEveryMember() {
			all = append(all, r.Approvers...)
		}
	}
	if len(all) == 0 {
		return nil
	}
	return NormalizeApprovers(all)
}

// ApplyApproverRules records rules on mr and derives from them Approvers
// (RequiredApprovers) and each reviewer's IsApprover flag.
func (mr *MergeRequest) ApplyApproverRules(rules []ApproverRule) {
	mr.ApproverRules = rules
	mr.Approvers = RequiredApprovers(rules)
	for i := range mr.Reviewers {
		mr.Reviewers[i].IsApprover = slices.Contains(mr.Approvers, mr.Reviewers[i].Username)
	}
}

// SetManagedApprovers makes usernames the managed rule's approvers, every one
// of them required, adding the managed rule when mr has none, and recomputes
// Approvers and every reviewer's IsApprover flag from the resulting rules.
func (mr *MergeRequest) SetManagedApprovers(usernames []string) {
	set := NormalizeApprovers(usernames)
	found := false
	rules := make([]ApproverRule, 0, len(mr.ApproverRules)+1)
	for _, r := range mr.ApproverRules {
		if r.Managed && !found {
			found = true
			r.Approvers = set
			r.ApprovalsRequired = len(set)
			r.UserIDs = idsOf(r.UserIDs, set)
		}
		rules = append(rules, r)
	}
	if !found {
		rules = append(rules, ApproverRule{
			Name: ManagedApproverRuleName, Approvers: set, ApprovalsRequired: len(set), Managed: true,
		})
	}
	mr.ApplyApproverRules(rules)
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

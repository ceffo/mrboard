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
	// Members are the users added to the rule, sorted, when the read that
	// produced the rule carries them; nil otherwise. GitLab leaves members who
	// cannot approve this MR (its author or committers, when the project
	// prevents their approval) out of Approvers, but not out of Members.
	Members []string
	// UserIDs maps each approver and member to their user ID when the read
	// that produced the rule carries IDs; nil otherwise.
	UserIDs map[string]int64
	// Managed marks the rule mrboard writes. mrboard never changes any other
	// rule: their approvers are locked as far as mrboard is concerned.
	Managed bool
}

// ManagedApprovers returns the usernames on mr's managed rule, sorted; nil when
// the MR has none. They are the rule's Members when the read carried them, so
// a member GitLab does not let approve this MR is still one: writing the rule
// back keeps them.
func (mr MergeRequest) ManagedApprovers() []string {
	for _, r := range mr.ApproverRules {
		if r.Managed {
			return NormalizeApprovers(append(slices.Clone(r.Members), r.Approvers...))
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
		if r.Managed {
			continue
		}
		for _, u := range r.RequiredMembers() {
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

// RequiredMembers returns the approvers of r when every one of them must
// approve; nil otherwise. Approvers are GitLab's eligible approvers: GitLab has
// already left out anyone who cannot approve this MR.
//
// An unmanaged rule requires every approver when it needs at least one
// approval and at least as many as it has approvers. Needing more than it has
// approvers is the case where GitLab left ineligible members out: every
// remaining one must still approve. A rule needing fewer approvals than it has
// approvers, or none, makes no one required.
//
// The managed rule's approvers are always required: mrboard writes it needing
// every member, and a count lowered by hand in GitLab does not demote them.
func (r ApproverRule) RequiredMembers() []string {
	if len(r.Approvers) == 0 {
		return nil
	}
	if r.Managed || (r.ApprovalsRequired > 0 && r.ApprovalsRequired >= len(r.Approvers)) {
		return slices.Clone(r.Approvers)
	}
	return nil
}

// RequiredApprovers returns the required members (ApproverRule.RequiredMembers)
// of every rule, sorted and deduplicated; nil when there are none.
func RequiredApprovers(rules []ApproverRule) []string {
	var all []string
	for _, r := range rules {
		all = append(all, r.RequiredMembers()...)
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
			r.Members = set
			r.ApprovalsRequired = len(set)
			r.UserIDs = idsOf(r.UserIDs, set)
		}
		rules = append(rules, r)
	}
	if !found {
		rules = append(rules, ApproverRule{
			Name: ManagedApproverRuleName, Approvers: set, Members: set, ApprovalsRequired: len(set), Managed: true,
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

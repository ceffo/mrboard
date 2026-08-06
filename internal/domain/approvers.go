package domain

// ApproversConflict reports whether two MRs' "Approvers" rule memberships
// differ. Compares MergeRequest.Approvers — the full eligible-approver
// roster — not who currently happens to be assigned as a reviewer, so two
// sibling MRs with an identical approval rule but different reviewers
// added so far are correctly reported as not conflicting. Used to warn
// (never block) when editing reviewers across a group of sibling MRs that
// share a JIRA key but started out with different approver rules.
func ApproversConflict(a, b MergeRequest) bool {
	setA := stringSet(a.Approvers)
	setB := stringSet(b.Approvers)
	if len(setA) != len(setB) {
		return true
	}
	for username := range setA {
		if !setB[username] {
			return true
		}
	}
	return false
}

func stringSet(usernames []string) map[string]bool {
	set := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		set[u] = true
	}
	return set
}

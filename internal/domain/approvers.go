package domain

// ApproversConflict reports whether two MRs' current approver sets differ.
// Only usernames flagged IsApprover are compared — reviewer state, waiting
// time, and any other field are irrelevant. Used to warn (never block) when
// editing reviewers across a group of sibling MRs that share a JIRA key but
// started out with different approver rosters.
func ApproversConflict(a, b MergeRequest) bool {
	setA := approverSet(a)
	setB := approverSet(b)
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

// approverSet returns the set of usernames flagged IsApprover on the MR.
func approverSet(mr MergeRequest) map[string]bool {
	set := make(map[string]bool, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			set[r.Username] = true
		}
	}
	return set
}

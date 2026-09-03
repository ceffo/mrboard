package domain

// AutoAssignCandidates decides whether mr should have the whole team assigned
// as reviewers, per docs/adr/0009's four criteria: authored by a teamRoster
// member, a ticket key in its title, no reviewers yet, and not a draft. When
// mr qualifies, it returns every teamRoster member except mr's own author and
// the ticket key that made it eligible, with ok set to true.
//
// ok is also false when mr meets all four criteria but teamRoster contains
// nothing besides mr's own author: SetReviewers treats an empty ID slice as
// "clear all reviewers," so returning an empty, non-nil slice here would turn
// a no-op into a destructive write.
func AutoAssignCandidates(
	mr MergeRequest, teamRoster []User, matcher TicketKeyMatcher,
) (reviewers []User, issueKey string, ok bool) {
	if !isTeamMember(mr.Author, teamRoster) {
		return nil, "", false
	}
	issueKey = matcher.ExtractFromTitle(mr.Title)
	if issueKey == "" {
		return nil, "", false
	}
	if len(mr.Reviewers) != 0 {
		return nil, "", false
	}
	if mr.Phase == PhaseDraft {
		return nil, "", false
	}
	for _, u := range teamRoster {
		if u.Username != mr.Author {
			reviewers = append(reviewers, u)
		}
	}
	if len(reviewers) == 0 {
		return nil, "", false
	}
	return reviewers, issueKey, true
}

func isTeamMember(username string, teamRoster []User) bool {
	for _, u := range teamRoster {
		if u.Username == username {
			return true
		}
	}
	return false
}

// Usernames extracts the Username field from a slice of Users, in order.
func Usernames(users []User) []string {
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Username
	}
	return names
}

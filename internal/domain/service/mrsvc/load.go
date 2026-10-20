package mrsvc

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// LoadGrouping is the axis the review load tree is grouped by (docs/adr/0015).
type LoadGrouping int

// LoadGrouping values.
const (
	LoadByPerson LoadGrouping = iota // one group per approver, each MR they approve beneath
	LoadByTicket                     // one group per ticket, each MR once beneath
)

// LoadSort orders the groups of a review load and the lines within each group.
type LoadSort int

// LoadSort values.
const (
	LoadSortPerson LoadSort = iota
	LoadSortTicket
	LoadSortSize
)

// LoadOptions are the inputs of ComputeLoad.
type LoadOptions struct {
	// SprintStart bounds merged MRs: one merged before it is not in the load.
	SprintStart time.Time
	// SprintKeys are the ticket keys of the sprint. An MR whose ticket key is not
	// among them is not in the load; an MR with no ticket key goes to the
	// no-ticket group.
	SprintKeys map[string]bool
	// KeyMatcher extracts an MR's ticket key from its title.
	KeyMatcher domain.TicketKeyMatcher
	// Team is the roster: each member gets a person group. Approvers outside it
	// are aggregated into the "others" group.
	Team []string
	// Filter carries the board's header filters. Every one applies except the
	// sprint filter and the board sort, with two differences: phases narrow open
	// MRs only, and an excluded reviewer loses their person group instead of
	// hiding the MRs they review.
	Filter FilterOptions
	// Exclusions and Thresholds measure and label each MR's review size.
	Exclusions domain.SizeExclusions
	Thresholds domain.SizeThresholds

	Group LoadGrouping
	Sort  LoadSort
	// RemainingOnly hides a person's line once they approved the MR or it merged.
	RemainingOnly bool
}

// LoadTotal is a sum of review sizes. Partial is true when at least one summed
// size was unknown: Lines then sums the known sizes only and reads as "Lines+".
type LoadTotal struct {
	Lines   int
	Partial bool
}

func (t *LoadTotal) add(s domain.ReviewSize) {
	if !s.Known {
		t.Partial = true
		return
	}
	t.Lines += s.Lines
}

// LoadApprover is one approver of an MR line.
type LoadApprover struct {
	Username string
	Name     string
	Approved bool
	// InTeam is false for an approver outside the roster, credited to "others".
	InTeam bool
}

// LoadLine is one MR line of a load group.
type LoadLine struct {
	MR     domain.MergeRequest
	Ticket string // "" when the title has no ticket key
	Size   domain.ReviewSize
	Bucket domain.SizeBucket
	Draft  bool
	Merged bool
	// Approvers are every approver of the MR, in reviewer order.
	Approvers []LoadApprover
	// Approver is the person the line is credited to in a person group; empty in
	// a ticket group.
	Approver string
	// Approved is whether Approver approved the MR; false in a ticket group.
	Approved bool
}

// CoApprovers returns the line's approvers other than its credited Approver.
func (l LoadLine) CoApprovers() []LoadApprover {
	out := make([]LoadApprover, 0, len(l.Approvers))
	for _, a := range l.Approvers {
		if a.Username != l.Approver {
			out = append(out, a)
		}
	}
	return out
}

// LoadGroup is a heading of the load tree and the MR lines beneath it.
type LoadGroup struct {
	// Key is the approver's username or the ticket key; empty for a catch-all.
	Key  string
	Name string // approver display name or ticket key; empty for a catch-all
	// CatchAll marks the "others" person group or the "(no ticket)" group. A
	// catch-all group is always last.
	CatchAll bool
	// Total is the group's load. A person group credits each line's full size;
	// a ticket group counts each MR once.
	Total LoadTotal
	Lines []LoadLine
}

// Load is the computed review load of a sprint, ready to render.
type Load struct {
	Group         LoadGrouping
	Sort          LoadSort
	RemainingOnly bool
	Groups        []LoadGroup
	// TeamTotal counts each MR shown once, so person totals can exceed it.
	TeamTotal LoadTotal
	// MRCount is the number of MRs shown; PopulationCount the sprint's MRs
	// before the header filters ran.
	MRCount         int
	PopulationCount int
}

// ComputeLoad derives the review load of a sprint from open and merged MRs
// (docs/adr/0015, "Crediting review size to approvers" and "Load population").
// An MR present twice (open in one listing, merged in another) counts once, as
// merged.
func ComputeLoad(mrs []domain.MergeRequest, opts LoadOptions) Load {
	population := loadPopulation(dedupMerged(mrs), opts)
	shown := filterForLoad(population, opts)

	team := make(map[string]bool, len(opts.Team))
	for _, u := range opts.Team {
		team[u] = true
	}
	lines := make([]LoadLine, 0, len(shown))
	for _, mr := range shown {
		lines = append(lines, newLoadLine(mr, opts, team))
	}

	out := Load{
		Group:           opts.Group,
		Sort:            opts.Sort,
		RemainingOnly:   opts.RemainingOnly,
		PopulationCount: len(population),
	}
	for _, l := range lines {
		if opts.RemainingOnly && !l.remaining() {
			continue
		}
		out.MRCount++
		out.TeamTotal.add(l.Size)
	}
	if opts.Group == LoadByTicket {
		out.Groups = groupByTicket(lines, opts)
	} else {
		out.Groups = groupByPerson(lines, mrs, opts)
	}
	sortGroups(out.Groups, opts.Sort)
	for i := range out.Groups {
		sortLines(out.Groups[i].Lines, opts.Sort)
	}
	return out
}

// dedupMerged keeps one MR per key, preferring a merged copy.
func dedupMerged(mrs []domain.MergeRequest) []domain.MergeRequest {
	idx := make(map[domain.MRKey]int, len(mrs))
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		i, seen := idx[mr.Key()]
		if !seen {
			idx[mr.Key()] = len(out)
			out = append(out, mr)
			continue
		}
		if mr.Merged() && !out[i].Merged() {
			out[i] = mr
		}
	}
	return out
}

// loadPopulation keeps the MRs of the sprint: ticketed in the sprint or
// unticketed, and open or merged on or after the sprint start.
func loadPopulation(mrs []domain.MergeRequest, opts LoadOptions) []domain.MergeRequest {
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		if mr.Merged() && mr.MergedAt.Before(opts.SprintStart) {
			continue
		}
		if k := opts.KeyMatcher.ExtractFromTitle(mr.Title); k != "" && !opts.SprintKeys[k] {
			continue
		}
		out = append(out, mr)
	}
	return out
}

// filterForLoad applies the header filters that narrow the MR set.
func filterForLoad(mrs []domain.MergeRequest, opts LoadOptions) []domain.MergeRequest {
	f := opts.Filter
	f.KeyMatcher = opts.KeyMatcher
	mrs = filterByMyView(mrs, f)
	mrs = filterByAssignees(mrs, f)
	mrs = filterByTicket(mrs, f)
	if len(f.Phases) == 0 {
		return mrs
	}
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		if mr.Merged() || f.Phases[mr.Phase] {
			out = append(out, mr)
		}
	}
	return out
}

func newLoadLine(mr domain.MergeRequest, opts LoadOptions, team map[string]bool) LoadLine {
	size := mr.MeasureReviewSize(opts.Exclusions).Size
	l := LoadLine{
		MR:     mr,
		Ticket: opts.KeyMatcher.ExtractFromTitle(mr.Title),
		Size:   size,
		Bucket: opts.Thresholds.Bucket(size),
		Merged: mr.Merged(),
		Draft:  !mr.Merged() && mr.Phase == domain.PhaseDraft,
	}
	for _, r := range mr.Reviewers {
		if !r.IsApprover {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.Username
		}
		l.Approvers = append(l.Approvers, LoadApprover{
			Username: r.Username,
			Name:     name,
			Approved: r.State == domain.ReviewerApproved,
			InTeam:   team[r.Username],
		})
	}
	return l
}

// remaining reports whether a ticket-group line still has approval left: the MR
// is not merged and not every approver approved. An MR without approvers is
// remaining while open.
func (l LoadLine) remaining() bool {
	if l.Merged {
		return false
	}
	for _, a := range l.Approvers {
		if !a.Approved {
			return true
		}
	}
	return len(l.Approvers) == 0
}

func groupByTicket(lines []LoadLine, opts LoadOptions) []LoadGroup {
	idx := make(map[string]int)
	var groups []LoadGroup
	for _, l := range lines {
		if opts.RemainingOnly && !l.remaining() {
			continue
		}
		i, ok := idx[l.Ticket]
		if !ok {
			i = len(groups)
			idx[l.Ticket] = i
			groups = append(groups, LoadGroup{Key: l.Ticket, Name: l.Ticket, CatchAll: l.Ticket == ""})
		}
		groups[i].Lines = append(groups[i].Lines, l)
		groups[i].Total.add(l.Size)
	}
	return groups
}

// groupByPerson credits every line's full size to each of its approvers. Each
// roster member not excluded by the reviewer filter gets a group, empty or not;
// approvers outside the roster share the "others" group, present when it has
// lines.
func groupByPerson(lines []LoadLine, all []domain.MergeRequest, opts LoadOptions) []LoadGroup {
	excluded := make(map[string]bool, len(opts.Filter.ExcludedReviewers))
	for _, u := range opts.Filter.ExcludedReviewers {
		excluded[u] = true
	}
	names := BuildUserMap(all)
	idx := make(map[string]int)
	var groups []LoadGroup
	for _, u := range opts.Team {
		if excluded[u] {
			continue
		}
		if _, dup := idx[u]; dup {
			continue
		}
		idx[u] = len(groups)
		groups = append(groups, LoadGroup{Key: u, Name: DisplayName(names, u)})
	}
	var others LoadGroup
	others.CatchAll = true
	for _, l := range lines {
		for _, a := range l.Approvers {
			if excluded[a.Username] || (opts.RemainingOnly && (l.Merged || a.Approved)) {
				continue
			}
			credited := l
			credited.Approver = a.Username
			credited.Approved = a.Approved
			g := &others
			if i, ok := idx[a.Username]; ok && a.InTeam {
				g = &groups[i]
			}
			g.Lines = append(g.Lines, credited)
			g.Total.add(l.Size)
		}
	}
	if len(others.Lines) > 0 {
		groups = append(groups, others)
	}
	return groups
}

// sortGroups orders groups by key, or by total (heaviest first) for a size
// sort. Catch-all groups always come last.
func sortGroups(groups []LoadGroup, by LoadSort) {
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.CatchAll != b.CatchAll {
			return b.CatchAll
		}
		if by == LoadSortSize && a.Total.Lines != b.Total.Lines {
			return a.Total.Lines > b.Total.Lines
		}
		if a.Name != b.Name {
			return lessTicketOrName(a.Name, b.Name)
		}
		return a.Key < b.Key
	})
}

// sortLines orders lines by size (heaviest first, unknown last), by ticket (no
// ticket last), or by person (first approver, then MR). Ties fall back to
// repo!iid, then the credited approver.
func sortLines(lines []LoadLine, by LoadSort) {
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		switch by {
		case LoadSortSize:
			if a.Size.Known != b.Size.Known {
				return a.Size.Known
			}
			if a.Size.Lines != b.Size.Lines {
				return a.Size.Lines > b.Size.Lines
			}
		case LoadSortTicket:
			if a.Ticket != b.Ticket {
				if a.Ticket == "" || b.Ticket == "" {
					return b.Ticket == ""
				}
				return lessTicketOrName(a.Ticket, b.Ticket)
			}
		case LoadSortPerson:
			if fa, fb := firstApproverName(a), firstApproverName(b); a.Approver == "" && fa != fb {
				return strings.ToLower(fa) < strings.ToLower(fb)
			}
		}
		if a.MR.ProjectPath != b.MR.ProjectPath {
			return a.MR.ProjectPath < b.MR.ProjectPath
		}
		if a.MR.IID != b.MR.IID {
			return a.MR.IID < b.MR.IID
		}
		return a.Approver < b.Approver
	})
}

func firstApproverName(l LoadLine) string {
	if len(l.Approvers) == 0 {
		return ""
	}
	return l.Approvers[0].Name
}

// lessTicketOrName compares case-insensitively, ordering ticket keys of one
// project by number ("OD-9" before "OD-10").
func lessTicketOrName(a, b string) bool {
	pa, na, oka := splitTicketKey(a)
	pb, nb, okb := splitTicketKey(b)
	if oka && okb && pa == pb && na != nb {
		return na < nb
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

func splitTicketKey(k string) (string, int, bool) {
	i := strings.LastIndexByte(k, '-')
	if i <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(k[i+1:])
	if err != nil {
		return "", 0, false
	}
	return k[:i], n, true
}

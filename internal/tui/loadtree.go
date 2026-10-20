package tui

import (
	"fmt"
	"strings"
	"time"

	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// LoadSprint is the sprint a rendered load covers.
type LoadSprint struct {
	Name       string
	Start, End time.Time // End is zero when the tracker does not report it
}

// NewLoadStyles builds the style set for rendering a load outside the board. An
// empty themeName selects the default theme; a mode other than "light" renders
// for a dark background, since a one-shot command has no terminal to query.
func NewLoadStyles(themeName, mode string) Styles {
	return NewStyles(LoadThemeByName(themeName), mode != themeModeLight)
}

// RenderLoad draws a review load as a tree: a header line, one block per group
// carrying its total with an MR line beneath each entry, and footnotes that
// explain the figures (docs/adr/0015, "CLI output layout"). A zero Styles
// renders plain text.
func RenderLoad(load mrsvc.Load, sprint LoadSprint, st Styles) string {
	var b strings.Builder
	b.WriteString(loadHeader(load, sprint, st))
	b.WriteString("\n")
	if load.MRCount == 0 {
		b.WriteString("\n" + st.LoadNote.Render("No MRs in scope.") + "\n")
	}
	for _, g := range load.Groups {
		b.WriteString("\n")
		b.WriteString(loadGroupHeading(g, load, st))
		b.WriteString("\n")
		writeLoadLines(&b, g, load, st)
	}
	for i, note := range loadFootnotes(load) {
		if i == 0 {
			b.WriteString("\n")
		}
		b.WriteString(st.LoadNote.Render(note) + "\n")
	}
	return b.String()
}

// loadHeader states the sprint, the MRs in scope and the scope. The count is the
// MRs the total sums, not the lines drawn: in person grouping an MR with no
// approver is counted but sits on no person line.
func loadHeader(load mrsvc.Load, sprint LoadSprint, st Styles) string {
	name := sprint.Name
	if name == "" {
		name = "Sprint"
	}
	parts := []string{st.LoadHeading.Render(name)}
	if dates := sprintDates(sprint); dates != "" {
		parts = append(parts, st.LoadMeta.Render(dates))
	}
	count := fmt.Sprintf("%d %s in scope", load.MRCount, plural(load.MRCount, "MR", "MRs"))
	if load.PopulationCount != load.MRCount {
		count += fmt.Sprintf(" (of %d in the sprint)", load.PopulationCount)
	}
	parts = append(parts, st.LoadMeta.Render(count))
	scope := "all"
	if load.RemainingOnly {
		scope = "remaining only"
	}
	parts = append(parts, st.LoadMeta.Render(scope))
	return strings.Join(parts, st.LoadMeta.Render(" · "))
}

func sprintDates(s LoadSprint) string {
	const layout = "Jan 2"
	switch {
	case s.Start.IsZero():
		return ""
	case s.End.IsZero():
		return "from " + s.Start.Format(layout)
	}
	return s.Start.Format(layout) + " – " + s.End.Format(layout)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func loadGroupHeading(g mrsvc.LoadGroup, load mrsvc.Load, st Styles) string {
	name := g.Name
	if g.CatchAll {
		name = "others"
		if load.Group == mrsvc.LoadByTicket {
			name = "(no ticket)"
		}
	}
	return st.LoadHeading.Render(name) + "  " + st.LoadTotal.Render(formatLoadTotal(g.Total)+" lines")
}

// formatLoadTotal marks a total partial, as "1240+", when a summed size was unknown.
func formatLoadTotal(t mrsvc.LoadTotal) string {
	if t.Partial {
		return fmt.Sprintf("%d+", t.Lines)
	}
	return fmt.Sprintf("%d", t.Lines)
}

// loadRow holds the plain text of each column of an MR line, so columns can be
// padded to their widest cell before styling.
type loadRow struct {
	ref, ticket, size, bucket, who, state, title string
	line                                         mrsvc.LoadLine
}

func writeLoadLines(b *strings.Builder, g mrsvc.LoadGroup, load mrsvc.Load, st Styles) {
	if len(g.Lines) == 0 {
		b.WriteString("  " + st.LoadNote.Render("no MRs") + "\n")
		return
	}
	byTicket := load.Group == mrsvc.LoadByTicket
	rows := make([]loadRow, 0, len(g.Lines))
	var w loadRow
	for _, l := range g.Lines {
		r := loadRow{
			ref:    mrRef(l.MR),
			ticket: l.Ticket,
			size:   sizeText(l.Size),
			bucket: string(l.Bucket),
			title:  l.MR.Title,
			line:   l,
		}
		if byTicket {
			r.who = approversText(l)
			r.state = stateText(l, false)
		} else {
			r.who = coApproversText(l)
			r.state = stateText(l, true)
		}
		rows = append(rows, r)
		w.ref = widest(w.ref, r.ref)
		w.ticket = widest(w.ticket, r.ticket)
		w.size = widest(w.size, r.size)
		w.bucket = widest(w.bucket, r.bucket)
		w.who = widest(w.who, r.who)
		w.state = widest(w.state, r.state)
	}
	for _, r := range rows {
		cells := []string{"  " + st.MRNumberBang.Render(pad(r.ref, w.ref))}
		if !byTicket {
			cells = append(cells, st.LoadMeta.Render(pad(r.ticket, w.ticket)))
		}
		cells = append(cells,
			st.LoadHeading.Render(padLeft(r.size, w.size)),
			paint(bucketStyle(r.line.Bucket, st), pad(r.bucket, w.bucket)),
			paint(st.LoadMeta, pad(r.who, w.who)),
			paint(stateStyle(r.line, byTicket, st), pad(r.state, w.state)),
			st.LoadMeta.Render(r.title),
		)
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
}

// paint styles s, leaving a blank cell unstyled so it adds no escape codes.
func paint(style lip.Style, s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return style.Render(s)
}

func mrRef(mr domain.MergeRequest) string {
	repo := mr.ProjectPath
	if i := strings.LastIndexByte(repo, '/'); i >= 0 {
		repo = repo[i+1:]
	}
	return fmt.Sprintf("%s!%d", repo, mr.IID)
}

func sizeText(s domain.ReviewSize) string {
	if !s.Known {
		return "?"
	}
	return fmt.Sprintf("%d", s.Lines)
}

// coApproversText lists the other approvers of a person-group line: roster
// members by username, everyone outside the roster as one "+others".
func coApproversText(l mrsvc.LoadLine) string {
	var parts []string
	others := false
	for _, a := range l.CoApprovers() {
		if a.InTeam {
			parts = append(parts, "+"+a.Username)
		} else {
			others = true
		}
	}
	if others {
		parts = append(parts, "+others")
	}
	return strings.Join(parts, " ")
}

// approversText lists every approver of a ticket-group line, ✓ marking those who
// approved.
func approversText(l mrsvc.LoadLine) string {
	parts := make([]string, 0, len(l.Approvers))
	for _, a := range l.Approvers {
		if a.Approved {
			parts = append(parts, "✓"+a.Username)
		} else {
			parts = append(parts, a.Username)
		}
	}
	if len(parts) == 0 {
		return "no approver"
	}
	return strings.Join(parts, " ")
}

// stateText names the MR's state: draft, merged and, in a person group, whether
// the credited approver approved. A ticket group shows approvals per approver.
func stateText(l mrsvc.LoadLine, personal bool) string {
	var parts []string
	if l.Draft {
		parts = append(parts, "draft")
	}
	if l.Merged {
		parts = append(parts, "merged")
	}
	if personal && l.Approved {
		parts = append(parts, "✓ approved")
	}
	return strings.Join(parts, " ")
}

func stateStyle(l mrsvc.LoadLine, byTicket bool, st Styles) lip.Style {
	switch {
	case l.Merged:
		return st.LoadMerged
	case l.Draft:
		return st.LoadDraft
	case !byTicket && l.Approved:
		return st.LoadApproved
	}
	return st.LoadMeta
}

func bucketStyle(b domain.SizeBucket, st Styles) lip.Style {
	switch b {
	case domain.SizeBucketS:
		return st.LoadBucketS
	case domain.SizeBucketM:
		return st.LoadBucketM
	case domain.SizeBucketL:
		return st.LoadBucketL
	case domain.SizeBucketXL:
		return st.LoadBucketXL
	}
	return st.LoadBucketUnknown
}

// loadFootnotes explains the figures that would otherwise mislead: person loads
// that add up to more than the team total, and MRs counted but on no person line.
func loadFootnotes(load mrsvc.Load) []string {
	if load.Group != mrsvc.LoadByPerson {
		return nil
	}
	var notes []string
	sum := 0
	onLine := make(map[domain.MRKey]bool)
	for _, g := range load.Groups {
		sum += g.Total.Lines
		for _, l := range g.Lines {
			onLine[l.MR.Key()] = true
		}
	}
	if sum > load.TeamTotal.Lines {
		notes = append(notes, fmt.Sprintf(
			"Every approver is credited an MR's full size, so person loads add up to more than the team total "+
				"(%s lines, each MR counted once).", formatLoadTotal(load.TeamTotal)))
	}
	if none := load.MRCount - len(onLine); none > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d %s on no person line (no approver, or only excluded ones) — counted in the header and the team total.",
			none, plural(none, "MR is", "MRs are")))
	}
	return notes
}

func widest(cur, s string) string {
	if lip.Width(s) > lip.Width(cur) {
		return s
	}
	return cur
}

func pad(s, like string) string {
	if n := lip.Width(like) - lip.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func padLeft(s, like string) string {
	if n := lip.Width(like) - lip.Width(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

package demoadpt

import (
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ceffo/mrboard/internal/domain"
)

//go:embed fixture/board.yaml
var fixtureFS embed.FS

const fixtureSchema = 1

// fixtureFile mirrors fixture/board.yaml. Field names deliberately describe
// capabilities (issue_types, merge_status) rather than any vendor, so the
// fixture reads as demo data for a review board and not for one provider.
type fixtureFile struct {
	Schema             int               `yaml:"schema"`
	SnapshotWrittenAgo string            `yaml:"snapshot_written_ago"`
	People             []fixturePerson   `yaml:"people"`
	Projects           []fixtureProject  `yaml:"projects"`
	IssueTypes         map[string]string `yaml:"issue_types"`
	SprintTicketKeys   []string          `yaml:"sprint_ticket_keys"`
	MergeRequests      []fixtureMR       `yaml:"merge_requests"`
}

type fixturePerson struct {
	Username string `yaml:"username"`
	Name     string `yaml:"name"`
	UserID   int64  `yaml:"user_id"`
}

type fixtureProject struct {
	ID      int64    `yaml:"id"`
	Path    string   `yaml:"path"`
	Members []string `yaml:"members"`
}

type fixtureReviewer struct {
	Username    string `yaml:"username"`
	Approver    bool   `yaml:"approver"`
	State       string `yaml:"state"`
	WaitingAgo  string `yaml:"waiting_ago"`
	ApprovedAgo string `yaml:"approved_ago"`
}

type fixtureNote struct {
	Author     string `yaml:"author"`
	Body       string `yaml:"body"`
	CreatedAgo string `yaml:"created_ago"`
}

type fixtureThread struct {
	Resolved bool          `yaml:"resolved"`
	Notes    []fixtureNote `yaml:"notes"`
}

type fixtureFileDiff struct {
	OldPath     string `yaml:"old_path"`
	NewPath     string `yaml:"new_path"`
	NewFile     bool   `yaml:"new_file"`
	DeletedFile bool   `yaml:"deleted_file"`
	RenamedFile bool   `yaml:"renamed_file"`
	TooLarge    bool   `yaml:"too_large"`
	Added       int    `yaml:"added"`
	Removed     int    `yaml:"removed"`
	Diff        string `yaml:"diff"`
}

type fixtureDiff struct {
	BaseSHA string            `yaml:"base_sha"`
	HeadSHA string            `yaml:"head_sha"`
	Files   []fixtureFileDiff `yaml:"files"`
}

type fixtureMR struct {
	ProjectID      int               `yaml:"project_id"`
	IID            int               `yaml:"iid"`
	ID             int               `yaml:"id"`
	Title          string            `yaml:"title"`
	Author         string            `yaml:"author"`
	Assignee       string            `yaml:"assignee"`
	Draft          bool              `yaml:"draft"`
	MergeStatus    string            `yaml:"merge_status"`
	SourceBranch   string            `yaml:"source_branch"`
	TargetBranch   string            `yaml:"target_branch"`
	CreatedAgo     string            `yaml:"created_ago"`
	UpdatedAgo     string            `yaml:"updated_ago"`
	OpenThreads    int               `yaml:"open_threads"`
	RoundTrips     int               `yaml:"round_trips"`
	ReviewerSource bool              `yaml:"reviewer_source"`
	Description    string            `yaml:"description"`
	Reviewers      []fixtureReviewer `yaml:"reviewers"`
	Threads        []fixtureThread   `yaml:"threads"`
	Diff           *fixtureDiff      `yaml:"diff"`
}

// reviewerStates maps the fixture's state names onto the domain enum. The names
// are exactly domain.ReviewerState.String()'s output; a test asserts this map
// covers every state so a new one cannot be added without updating the fixture
// vocabulary.
var reviewerStates = map[string]domain.ReviewerState{
	"not_started":         domain.ReviewerNotStarted,
	"commented":           domain.ReviewerCommented,
	"re_review_requested": domain.ReviewerReReviewRequested,
	"approved":            domain.ReviewerApproved,
}

// offsetPattern matches the fixture's `<n>d<n>h<n>m` age syntax. time.ParseDuration
// has no day unit and "2976h" is unreadable for "4 months", so the fixture uses
// the same vocabulary domain.FormatDuration prints.
var offsetPattern = regexp.MustCompile(`^(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?$`)

// parseOffset converts a fixture age like "124d4h" or "9h37m" into a duration.
// An empty string is a zero duration, which callers treat as "field unset".
func parseOffset(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	m := offsetPattern.FindStringSubmatch(s)
	if m == nil || (m[1] == "" && m[2] == "" && m[3] == "") {
		return 0, fmt.Errorf("demoadpt: malformed age offset %q, want <n>d<n>h<n>m", s)
	}
	part := func(raw string) time.Duration {
		if raw == "" {
			return 0
		}
		n, _ := strconv.Atoi(raw) //nolint:errcheck // the regexp guarantees digits
		return time.Duration(n)
	}
	return part(m[1])*24*time.Hour + part(m[2])*time.Hour + part(m[3])*time.Minute, nil
}

// loadFixture parses the embedded dataset and materialises it against bootAt,
// so every rendered age is measured from a single anchor and nothing drifts as
// the fixture ages in git.
func loadFixture(bootAt time.Time, baseURL string) (*dataset, error) {
	raw, err := fixtureFS.ReadFile("fixture/board.yaml")
	if err != nil {
		return nil, fmt.Errorf("demoadpt: read fixture: %w", err)
	}
	var f fixtureFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("demoadpt: parse fixture: %w", err)
	}
	if f.Schema != fixtureSchema {
		return nil, fmt.Errorf("demoadpt: fixture schema %d, want %d", f.Schema, fixtureSchema)
	}

	ago := func(s string) (time.Time, error) {
		d, err := parseOffset(s)
		if err != nil {
			return time.Time{}, err
		}
		if d == 0 {
			return time.Time{}, nil
		}
		return bootAt.Add(-d), nil
	}

	ds := &dataset{
		people:       make(map[string]fixturePerson, len(f.People)),
		peopleByID:   make(map[int64]fixturePerson, len(f.People)),
		projectPaths: make(map[int]string, len(f.Projects)),
		members:      make(map[int][]domain.ProjectMember, len(f.Projects)),
		issueTypes:   f.IssueTypes,
		sprintKeys:   f.SprintTicketKeys,
		threads:      make(map[domain.MRKey][]domain.Thread),
		diffs:        make(map[domain.MRKey]domain.MRDiff),
		drafts:       make(map[domain.MRKey]bool),
	}
	for _, p := range f.People {
		ds.people[p.Username] = p
		ds.peopleByID[p.UserID] = p
	}
	for _, pr := range f.Projects {
		ds.projectPaths[int(pr.ID)] = pr.Path
		for _, u := range pr.Members {
			p, ok := ds.people[u]
			if !ok {
				return nil, fmt.Errorf("demoadpt: project %d lists unknown member %q", pr.ID, u)
			}
			ds.members[int(pr.ID)] = append(ds.members[int(pr.ID)],
				domain.ProjectMember{UserID: p.UserID, Username: p.Username, Name: p.Name})
		}
	}

	if ds.snapshotWrittenAt, err = ago(f.SnapshotWrittenAgo); err != nil {
		return nil, err
	}

	for _, fm := range f.MergeRequests {
		mr, err := buildMR(fm, ds, baseURL, ago)
		if err != nil {
			return nil, err
		}
		key := mr.Key()
		ds.mrs = append(ds.mrs, mr)
		ds.drafts[key] = fm.Draft
		if fm.Diff != nil {
			ds.diffs[key] = buildDiff(*fm.Diff)
		} else {
			// Every MR needs a diff: the diff view is one keypress from any card,
			// and a card that answers "no files changed" reads as a broken demo.
			// Only the MRs whose diff is worth showing off spell one out.
			ds.diffs[key] = syntheticDiff(mr)
		}
		if len(fm.Threads) > 0 {
			th, err := buildThreads(fm.Threads, ago)
			if err != nil {
				return nil, err
			}
			ds.threads[key] = th
		}
	}
	return ds, nil
}

func buildMR(
	fm fixtureMR, ds *dataset, baseURL string,
	ago func(string) (time.Time, error),
) (domain.MergeRequest, error) {
	author, ok := ds.people[fm.Author]
	if !ok {
		return domain.MergeRequest{}, fmt.Errorf("demoadpt: MR !%d has unknown author %q", fm.IID, fm.Author)
	}
	path, ok := ds.projectPaths[fm.ProjectID]
	if !ok {
		return domain.MergeRequest{}, fmt.Errorf("demoadpt: MR !%d has unknown project %d", fm.IID, fm.ProjectID)
	}

	reviewers := make([]domain.ReviewerInfo, 0, len(fm.Reviewers))
	for _, fr := range fm.Reviewers {
		p, ok := ds.people[fr.Username]
		if !ok {
			return domain.MergeRequest{}, fmt.Errorf("demoadpt: MR !%d has unknown reviewer %q", fm.IID, fr.Username)
		}
		state, ok := reviewerStates[fr.State]
		if !ok {
			return domain.MergeRequest{}, fmt.Errorf("demoadpt: MR !%d reviewer %q has unknown state %q",
				fm.IID, fr.Username, fr.State)
		}
		waiting, err := ago(fr.WaitingAgo)
		if err != nil {
			return domain.MergeRequest{}, err
		}
		approved, err := ago(fr.ApprovedAgo)
		if err != nil {
			return domain.MergeRequest{}, err
		}
		reviewers = append(reviewers, domain.ReviewerInfo{
			Username: p.Username, Name: p.Name, State: state,
			WaitingSince: waiting, ApprovedAt: approved, IsApprover: fr.Approver,
		})
	}

	createdAt, err := ago(fm.CreatedAgo)
	if err != nil {
		return domain.MergeRequest{}, err
	}
	updatedAt, err := ago(fm.UpdatedAgo)
	if err != nil {
		return domain.MergeRequest{}, err
	}

	assignee, assigneeName := fm.Author, author.Name
	if fm.Assignee != "" {
		a, ok := ds.people[fm.Assignee]
		if !ok {
			return domain.MergeRequest{}, fmt.Errorf("demoadpt: MR !%d has unknown assignee %q", fm.IID, fm.Assignee)
		}
		assignee, assigneeName = a.Username, a.Name
	}
	target := fm.TargetBranch
	if target == "" {
		target = "main"
	}

	mr := domain.MergeRequest{
		ID: fm.ID, IID: fm.IID, ProjectID: fm.ProjectID,
		Title: fm.Title, Description: fm.Description,
		Author: author.Username, AuthorName: author.Name,
		Assignee: assignee, AssigneeName: assigneeName,
		ProjectPath:         path,
		WebURL:              fmt.Sprintf("%s/%s/-/merge_requests/%d", strings.TrimRight(baseURL, "/"), path, fm.IID),
		DetailedMergeStatus: fm.MergeStatus,
		SourceBranch:        fm.SourceBranch, TargetBranch: target,
		Reviewers: reviewers,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
		OpenThreads: fm.OpenThreads, RoundTripCount: fm.RoundTrips,
		ReviewerSource: fm.ReviewerSource,
	}
	applyDerivedFields(&mr, fm.Draft)
	return mr, nil
}

// applyDerivedFields recomputes Phase, WaitingSince and ReadyToMergeSince from
// the draft flag and reviewer set, in the same order gitlabadpt's mapper does.
// Called on load and again after every in-memory write, so promoting a reviewer
// to an approver moves the card between columns exactly as it would against a
// real instance.
//
// draft is passed in because domain.MergeRequest has no draft field — the phase
// is the only place that bit survives, so the dataset has to remember it.
func applyDerivedFields(mr *domain.MergeRequest, draft bool) {
	mr.Phase = domain.ClassifyPhase(draft, mr.DetailedMergeStatus == "mergeable", mr.Reviewers)
	mr.WaitingSince = domain.DeriveWaitingSince(mr.Phase, mr.Reviewers, mr.CreatedAt)
	mr.ReadyToMergeSince = time.Time{}
	if mr.Phase == domain.PhaseReadyToMerge {
		var latest time.Time
		for _, r := range mr.Reviewers {
			if r.State == domain.ReviewerApproved && r.ApprovedAt.After(latest) {
				latest = r.ApprovedAt
			}
		}
		mr.ReadyToMergeSince = latest
	}
}

func buildThreads(fts []fixtureThread, ago func(string) (time.Time, error)) ([]domain.Thread, error) {
	out := make([]domain.Thread, 0, len(fts))
	for _, ft := range fts {
		notes := make([]domain.DiscussionNote, 0, len(ft.Notes))
		for _, fn := range ft.Notes {
			at, err := ago(fn.CreatedAgo)
			if err != nil {
				return nil, err
			}
			notes = append(notes, domain.DiscussionNote{Author: fn.Author, Body: fn.Body, CreatedAt: at})
		}
		out = append(out, domain.Thread{Notes: notes, Resolved: ft.Resolved})
	}
	return out, nil
}

// Arbitrary primes, used only to spread the synthetic SHAs apart so two MRs do
// not display the same base/head refs. Nothing verifies them.
const (
	baseSHAFactor = 7919
	headSHAFactor = 104729
)

// Line counts of the synthetic diff body below, kept in sync by hand.
const (
	syntheticLinesAdded   = 8
	syntheticLinesRemoved = 1
)

// syntheticDiff builds a small, plausible one-file diff for an MR the fixture
// does not spell one out for, so pressing the diff key on any card shows
// something. Derived deterministically from the MR's own fields, so it stays
// stable across runs.
func syntheticDiff(mr domain.MergeRequest) domain.MRDiff {
	branch := mr.SourceBranch
	if branch == "" {
		branch = "topic"
	}
	path := "internal/" + strings.ReplaceAll(branch, "-", "_") + ".go"
	body := fmt.Sprintf(`@@ -18,6 +18,14 @@ package internal
 // %s
 func handle(req Request) (Response, error) {
-	return legacyHandle(req)
+	if err := req.Validate(); err != nil {
+		return Response{}, fmt.Errorf("%%s: %%w", req.Name, err)
+	}
+	return handleV2(req)
 }
+
+// handleV2 is the replacement path introduced by !%d.
+func handleV2(req Request) (Response, error) {
+	return Response{OK: true}, nil
+}
`, mr.Title, mr.IID)

	return domain.MRDiff{
		BaseSHA: fmt.Sprintf("%040x", mr.ID*baseSHAFactor),
		HeadSHA: fmt.Sprintf("%040x", mr.ID*headSHAFactor),
		Files: []domain.FileDiff{{
			OldPath: path, NewPath: path,
			Diff: body, LinesAdded: syntheticLinesAdded, LinesRemoved: syntheticLinesRemoved,
		}},
	}
}

func buildDiff(fd fixtureDiff) domain.MRDiff {
	files := make([]domain.FileDiff, 0, len(fd.Files))
	for _, ff := range fd.Files {
		old := ff.OldPath
		if old == "" {
			old = ff.NewPath
		}
		files = append(files, domain.FileDiff{
			OldPath: old, NewPath: ff.NewPath,
			NewFile: ff.NewFile, DeletedFile: ff.DeletedFile,
			RenamedFile: ff.RenamedFile, TooLarge: ff.TooLarge,
			Diff: ff.Diff, LinesAdded: ff.Added, LinesRemoved: ff.Removed,
		})
	}
	return domain.MRDiff{BaseSHA: fd.BaseSHA, HeadSHA: fd.HeadSHA, Files: files}
}

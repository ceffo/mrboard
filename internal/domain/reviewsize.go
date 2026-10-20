package domain

import (
	"fmt"
	"path"
	"strings"
)

// FileDiffStat is one file's line counts in an MR diff.
type FileDiffStat struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// DefaultSizeExcludeGlobs approximates the files GitLab collapses by default in an MR's
// Changes tab: lock files, vendored dependencies, minified assets, source maps and generated
// protobuf/Go code (docs/adr/0015, "Review size metric"). Path-based only: GitLab's
// content heuristics (e.g. a "Code generated ... DO NOT EDIT" header) cannot be reproduced
// from diff stats.
var DefaultSizeExcludeGlobs = []string{
	// lock files
	"*.lock",
	"package-lock.json",
	"npm-shrinkwrap.json",
	"pnpm-lock.yaml",
	"go.sum",
	"go.work.sum",
	// vendored dependencies
	"**/vendor/**",
	"**/node_modules/**",
	// minified assets and source maps
	"*.min.js",
	"*.min.css",
	"*.js.map",
	"*.css.map",
	// generated protobuf and Go code
	"*.pb.go",
	"*.pb.gw.go",
	"*_pb2.py",
	"*_pb2_grpc.py",
	"*.pb.cc",
	"*.pb.h",
	"zz_generated*.go",
}

// SizeExclusions decides which files of an MR diff are left out of its review size.
type SizeExclusions struct {
	globs     []string
	generated []GeneratedRule
}

// NewSizeExclusions combines the exclusion sources of docs/adr/0015: the built-in globs
// (when useDefaults), the configured globs, and the generated-file markings of the MR's
// project .gitattributes (ParseGeneratedRules). Every glob must have passed
// ValidateSizeGlob. A file is excluded when any glob matches it or the markings mark it
// generated; an unset marking never re-includes a file a glob excludes.
func NewSizeExclusions(useDefaults bool, configured []string, generated []GeneratedRule) SizeExclusions {
	var globs []string
	if useDefaults {
		globs = append(globs, DefaultSizeExcludeGlobs...)
	}
	globs = append(globs, configured...)
	return SizeExclusions{globs: globs, generated: generated}
}

// SizeExclusionsFor returns the exclusions that apply to an MR's diff. They can differ per
// MR: the generated-file rules come from the MR's project and target branch. Every reader of
// a review size (fetch output, the review load) measures through the same lookup, so they
// agree on each MR's size.
type SizeExclusionsFor func(MergeRequest) SizeExclusions

// UniformSizeExclusions applies e to every MR.
func UniformSizeExclusions(e SizeExclusions) SizeExclusionsFor {
	return func(MergeRequest) SizeExclusions { return e }
}

// Of returns the exclusions for mr; a nil lookup excludes nothing.
func (f SizeExclusionsFor) Of(mr MergeRequest) SizeExclusions {
	if f == nil {
		return SizeExclusions{}
	}
	return f(mr)
}

// Excludes reports whether the file at filePath is left out of the review size.
func (e SizeExclusions) Excludes(filePath string) bool {
	for _, g := range e.globs {
		if matchSizeGlob(g, filePath) {
			return true
		}
	}
	return markedGenerated(e.generated, filePath)
}

// ReviewSize is an MR's review size: added plus deleted lines over its counted files.
// Known is false when the size cannot be measured (stats not fetched, diff overflow); an
// unknown size is never read as zero.
type ReviewSize struct {
	Lines int
	Known bool
}

// UnknownReviewSize is the size of an MR whose diff stats are unavailable.
var UnknownReviewSize = ReviewSize{}

// SizeMeasurement is a review size together with the files that made it up.
type SizeMeasurement struct {
	Size     ReviewSize
	Counted  []FileDiffStat
	Excluded []FileDiffStat
}

// MeasureReviewSize computes the review size of a diff from its per-file stats.
// A nil stats slice means the stats are unknown; an empty non-nil slice is a known
// empty diff.
func (e SizeExclusions) MeasureReviewSize(stats []FileDiffStat) SizeMeasurement {
	if stats == nil {
		return SizeMeasurement{Size: UnknownReviewSize}
	}
	m := SizeMeasurement{Size: ReviewSize{Known: true}}
	for _, s := range stats {
		if e.Excludes(s.Path) {
			m.Excluded = append(m.Excluded, s)
			continue
		}
		m.Counted = append(m.Counted, s)
		m.Size.Lines += s.Additions + s.Deletions
	}
	return m
}

// SizeBucket is the coarse label of a review size. It is a label only: totals and sorting
// always use the raw line count.
type SizeBucket string

// SizeBucket values. SizeBucketUnknown labels an unknown size.
const (
	SizeBucketUnknown SizeBucket = "?"
	SizeBucketS       SizeBucket = "S"
	SizeBucketM       SizeBucket = "M"
	SizeBucketL       SizeBucket = "L"
	SizeBucketXL      SizeBucket = "XL"
)

// SizeThresholds are the inclusive upper bounds of the S, M and L buckets; anything above L
// is XL. Valid thresholds are positive and strictly increasing.
type SizeThresholds struct {
	S int
	M int
	L int
}

// DefaultSizeThresholds follow the ~400 LOC review-quality drop and Google's ~1250-line mark
// (docs/adr/0015).
var DefaultSizeThresholds = SizeThresholds{S: defaultSizeS, M: defaultSizeM, L: defaultSizeL}

const (
	defaultSizeS = 100
	defaultSizeM = 400
	defaultSizeL = 1000
)

// Validate reports whether t is positive and strictly increasing.
func (t SizeThresholds) Validate() error {
	if t.S <= 0 || t.S >= t.M || t.M >= t.L {
		return fmt.Errorf("size thresholds must be positive and increasing (s < m < l), got s=%d m=%d l=%d",
			t.S, t.M, t.L)
	}
	return nil
}

// Bucket labels size under t.
func (t SizeThresholds) Bucket(size ReviewSize) SizeBucket {
	switch {
	case !size.Known:
		return SizeBucketUnknown
	case size.Lines <= t.S:
		return SizeBucketS
	case size.Lines <= t.M:
		return SizeBucketM
	case size.Lines <= t.L:
		return SizeBucketL
	default:
		return SizeBucketXL
	}
}

// ValidateSizeGlob reports whether glob is a well-formed size-exclusion glob.
func ValidateSizeGlob(glob string) error {
	if strings.TrimSpace(glob) == "" {
		return fmt.Errorf("empty exclusion glob")
	}
	for _, seg := range strings.Split(strings.TrimSuffix(glob, "/"), "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("exclusion glob %q: %w", glob, err)
		}
	}
	return nil
}

// matchSizeGlob matches filePath against glob with .gitattributes-style rules: a glob with
// no slash matches the file's base name at any depth; otherwise (including a leading slash,
// which anchors "/foo.pb.go" to the root) it matches the whole path from the repository
// root, segment by segment, where "**" spans zero or more segments.
// A trailing slash ("vendor/") matches everything under that directory.
func matchSizeGlob(glob, filePath string) bool {
	filePath = strings.TrimPrefix(filePath, "/")
	anchored := strings.HasPrefix(glob, "/")
	glob = strings.TrimPrefix(glob, "/")
	if strings.HasSuffix(glob, "/") {
		glob += "**"
	}
	if !anchored && !strings.Contains(glob, "/") {
		return globMatch(glob, path.Base(filePath))
	}
	return matchSegments(strings.Split(glob, "/"), strings.Split(filePath, "/"))
}

func matchSegments(glob, segs []string) bool {
	for len(glob) > 0 {
		if glob[0] == "**" {
			rest := glob[1:]
			for i := 0; i <= len(segs); i++ {
				if matchSegments(rest, segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if !globMatch(glob[0], segs[0]) {
			return false
		}
		glob, segs = glob[1:], segs[1:]
	}
	return len(segs) == 0
}

// globMatch is path.Match with a malformed pattern treated as no match: exclusion globs are
// checked by ValidateSizeGlob when configuration loads.
func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

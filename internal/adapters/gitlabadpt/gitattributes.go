package gitlabadpt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/eko/gocache/lib/v4/store"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	ilog "github.com/ceffo/mrboard/internal/log"
	"github.com/ceffo/mrboard/pkg/diskstore"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// gqlGitAttributesBatchSize caps how many (project, ref) pairs go into one aliased
// repository.blobs query. GitLab rejects a query whose complexity exceeds 250
// (authenticated). GitLab scores every selected field 1 and a field that calls Gitaly 1
// more (see gqlDiffStatsBatchSize). One alias selects project, repository, blobs
// (Gitaly), nodes and rawTextBlob (Gitaly): 7. Budgeting 10 per alias covers resolver
// surcharges and the connection's per-item multiplier (first: 1); 20 aliases score at
// most 200 plus 3 for queryComplexity { score limit }, which the query requests so the
// debug log reports GitLab's real score.
const gqlGitAttributesBatchSize = 20

// GitAttributesTTL is how long a project's cached root .gitattributes is served before
// it is read again. Fixed: generated-file markings change rarely, and a cold fetch
// bypasses the cache.
const GitAttributesTTL = 24 * time.Hour

// gitAttrsKey identifies one root .gitattributes: a project at a target branch.
type gitAttrsKey struct {
	project string
	ref     string
}

// GitAttributesCache is the on-disk cache of each (project, target branch)'s root
// .gitattributes content. A missing file is cached as empty content: no rules.
type GitAttributesCache struct {
	cache *marshaler.Marshaler
}

// cachedGitAttributes is the cached value; Content is "" for a missing file.
type cachedGitAttributes struct {
	Content string
}

// NewGitAttributesCache returns a cache writing one file per entry under cfg.Dir.
func NewGitAttributesCache(cfg diskstore.Config) (*GitAttributesCache, error) {
	st, err := diskstore.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("gitlabadpt: gitattributes cache: %w", err)
	}
	return &GitAttributesCache{cache: marshaler.New(cache.New[any](st))}, nil
}

func (k gitAttrsKey) cacheKey() string { return "gitattributes_" + k.project + "@" + k.ref }

// get returns the cached content for k; ok is false on a miss, an expired or corrupt
// entry, or a nil cache.
func (c *GitAttributesCache) get(ctx context.Context, k gitAttrsKey) (content string, ok bool) {
	if c == nil {
		return "", false
	}
	var v cachedGitAttributes
	if _, err := c.cache.Get(ctx, k.cacheKey(), &v); err != nil {
		return "", false
	}
	return v.Content, true
}

// set writes content for k with GitAttributesTTL. A write failure is logged: the caller
// already holds the live content.
func (c *GitAttributesCache) set(ctx context.Context, k gitAttrsKey, content string) {
	if c == nil {
		return
	}
	err := c.cache.Set(ctx, k.cacheKey(), cachedGitAttributes{Content: content}, store.WithExpiration(GitAttributesTTL))
	if err != nil {
		ilog.FromContext(ctx).Warn("gitlab: gitattributes cache write failed",
			"project", k.project, "ref", k.ref, "error", err)
	}
}

// gitAttrsKeys maps every deduped survivor to the .gitattributes its review size reads:
// its project's, on its target branch. An MR without a project path or target branch
// has none.
func gitAttrsKeys(rest []*gl.BasicMergeRequest, gql []pkggitlab.GQLMergeRequest) map[mrKey]gitAttrsKey {
	keys := make(map[mrKey]gitAttrsKey, len(rest)+len(gql))
	for _, mr := range gql {
		if mr.Project.FullPath != "" && mr.TargetBranch != "" {
			keys[gqlMRKey(mr)] = gitAttrsKey{project: mr.Project.FullPath, ref: mr.TargetBranch}
		}
	}
	for _, mr := range rest {
		if p := projectPathFromRef(mr.References); p != "" && mr.TargetBranch != "" {
			keys[mrKey{ProjectID: int(mr.ProjectID), IID: int(mr.IID)}] = gitAttrsKey{project: p, ref: mr.TargetBranch}
		}
	}
	return keys
}

// gitAttributesStage returns the generated-file rules of every distinct key in keys.
// Unexpired cache entries are served unless cold; the rest are read in aliased batches
// of at most gqlGitAttributesBatchSize running concurrently, and cached. A failed batch
// or a project GitLab does not find leaves its keys without rules and uncached; a failed
// batch contributes one error.
func (a *GitLabAdapter) gitAttributesStage(
	ctx context.Context, keys map[mrKey]gitAttrsKey, cold bool,
) (map[gitAttrsKey][]domain.GeneratedRule, []error) {
	logger := ilog.FromContext(ctx)
	rules := make(map[gitAttrsKey][]domain.GeneratedRule)
	seen := make(map[gitAttrsKey]bool)
	var targets []gitAttrsKey
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if !cold {
			if content, ok := a.cfg.GitAttributes.get(ctx, k); ok {
				rules[k] = domain.ParseGeneratedRules(content)
				continue
			}
		}
		targets = append(targets, k)
	}
	logger.Info("gitlab: gitattributes plan", "cached", len(rules), "to_fetch", len(targets), "cold", cold)

	var errs []error
	var mu sync.Mutex
	var wg sync.WaitGroup
	for start := 0; start < len(targets); start += gqlGitAttributesBatchSize {
		batch := targets[start:min(start+gqlGitAttributesBatchSize, len(targets))]
		wg.Add(1)
		go func() {
			defer wg.Done()
			reqs := make([]pkggitlab.GitAttributesRequest, len(batch))
			for i, k := range batch {
				reqs[i] = pkggitlab.GitAttributesRequest{ProjectFullPath: k.project, Ref: k.ref}
			}
			results, err := a.client.FetchGitAttributesGraphQL(ctx, reqs)
			if err != nil {
				logger.Error("gitlab: gitattributes batch failed", "projects", len(batch), "error", err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("gitattributes batch of %d projects: %w", len(batch), err))
				mu.Unlock()
				return
			}
			for i, k := range batch {
				if i >= len(results) || !results[i].Found {
					continue
				}
				a.cfg.GitAttributes.set(ctx, k, results[i].Content)
				mu.Lock()
				rules[k] = domain.ParseGeneratedRules(results[i].Content)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return rules, errs
}

// applyGeneratedRules sets each MR's GeneratedRules from the rules of its key. An MR
// whose key has no rules (its read failed or its project was not found) keeps the rules
// the previous snapshot recorded for it on the same target branch; with none to keep,
// its rules are marked unknown, which makes its review size unknown rather than one
// that counts generated files.
func applyGeneratedRules(
	mrs []domain.MergeRequest, keys map[mrKey]gitAttrsKey, rules map[gitAttrsKey][]domain.GeneratedRule,
	previous []domain.MergeRequest,
) {
	prevByKey := make(map[mrKey]domain.MergeRequest, len(previous))
	for _, mr := range previous {
		prevByKey[mr.Key()] = mr
	}
	for i := range mrs {
		k, ok := keys[mrs[i].Key()]
		if !ok {
			continue
		}
		if r, read := rules[k]; read {
			mrs[i].GeneratedRules, mrs[i].GeneratedRulesUnknown = r, false
			continue
		}
		if prev, found := prevByKey[mrs[i].Key()]; found && prev.TargetBranch == k.ref {
			mrs[i].GeneratedRules, mrs[i].GeneratedRulesUnknown = prev.GeneratedRules, prev.GeneratedRulesUnknown
			continue
		}
		mrs[i].GeneratedRules, mrs[i].GeneratedRulesUnknown = nil, true
	}
}

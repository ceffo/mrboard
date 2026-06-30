# ADR-0003: JIRA Remote Issue Links

**Status**: Accepted

## Context

mrboard shows JIRA keys on cards and injects a back-link into the MR description
([internal/adapters/gitlabadpt/inject_jira.go](../../internal/adapters/gitlabadpt/inject_jira.go)).
The reverse direction — a reference from the JIRA issue back to the GitLab MR — is absent.
JIRA tickets have no visibility into which MRs implement them.

## Decision

Write JIRA Remote Issue Links automatically after every `FetchAll`. For each MR whose title
contains a JIRA key, call `POST /rest/api/3/issue/{key}/remotelink` with:

- `globalId`: `mrboard:{projectID}:{mrIID}` — GitLab project ID is stable across project
  renames and group transfers; using `projectPath` would orphan existing links on rename.
- `relationship`: `"mentioned in"`
- `object.title`: MR title, `object.url`: MR WebURL. No status field (mrboard only sees open MRs).

**Cache strategy** — to avoid polluting JIRA change history with no-op writes:

1. Session `sync.Map` on `jiraadpt.Adapter`: skip MRs already processed this session.
2. Persistent disk cache (same infrastructure as the issue-type cache): keyed by `globalId`,
   value = last-written title. Skip the JIRA call when the title is unchanged.
3. On disk-cache miss: `GET /rest/api/3/issue/{key}/remotelink?globalId=…` first. If JIRA
   already holds a link with a matching title, update the disk cache and skip the `POST`.
   Only write when the title actually differs. A 404 means no link exists yet — proceed with `POST`.

The trigger lives in the TUI after `FetchResultMsg`, not inside `gitlabadpt.FetchAll` — keeping
the GitLab adapter free of JIRA write dependencies. A new `jirasvc.JiraLinker` driven port
separates JIRA reads (`JiraEnricher`) from writes (`JiraLinker`); `jiraadpt.Adapter` implements
both. Write failures surface as TUI notifications; the session map entry is removed on error to
allow retry on the next refresh.

## Consequences

- The `globalId` format `mrboard:{projectID}:{mrIID}` is load-bearing: changing it orphans
  existing JIRA links and creates duplicates on the next fetch.
- The disk cache path (`$XDG_CACHE_HOME/mrboard/jira/remotelinks/`) is equally load-bearing.

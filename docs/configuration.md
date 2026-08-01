# Configuration

Complete reference for mrboard's config file. For a minimal starting point see the
[README](../README.md); for a copy-pasteable example see
[`mrboard.yaml.example`](../mrboard.yaml.example).

## Where the config is loaded from

First match wins:

1. the path given to `--config` / `-c`
2. `$XDG_CONFIG_HOME/mrboard/mrboard.yaml` (default `~/.config/mrboard/mrboard.yaml`)
3. `./mrboard.yaml` in the current directory

Note that the XDG path beats `./mrboard.yaml`. If you keep a scratch config in a project
directory it will be *ignored* whenever `~/.config/mrboard/mrboard.yaml` exists — pass
`--config` explicitly instead.

Only two settings can be overridden by the environment:

| Variable | Overrides |
| --- | --- |
| `GITLAB_TOKEN` | `gitlab.token` |
| `JIRA_TOKEN` | `jira.api_token` |

## Required settings

A config is rejected at startup unless it has `gitlab.url`, `gitlab.token` (or
`$GITLAB_TOKEN`), and at least one entry under `sources`.

```yaml
gitlab:
  url: https://gitlab.example.com
  token: glpat-xxx        # or $GITLAB_TOKEN

sources:
  - type: group
    ids: [my-team]
```

The token needs the **`api`** scope. `read_api` is not enough — the reviewer editor, the
batch editor, and the ticket back-link injection all write to GitLab.

## Sources

```yaml
sources:
  - type: group
    ids: [my-team, 4815]  # group paths or numeric IDs

  - type: user
    ids: [alice, bob]     # usernames
```

Mix as many `group` and `user` entries as you need; results from all of them are merged and
deduplicated. A `user` source also populates the roster used by the reviewer editor's "set
team" action.

## Top-level settings

| Key | Default | Meaning |
| --- | --- | --- |
| `gitlab.url` | *required* | Base URL of your GitLab instance |
| `gitlab.token` | *required* | PAT with `api` scope; or set `$GITLAB_TOKEN` |
| `gitlab.timeout` | `30s` | Per-fetch deadline, shared across all sources |
| `sources` | *required* | See above |
| `current_user` | — | Your GitLab username. Highlights your MRs and enables the "my view" toggle |
| `excluded_authors` | — | Usernames whose MRs are dropped entirely (bots, etc.) |
| `refresh_interval` | `60s` | Background auto-refresh cadence. `0` disables it; manual refresh always works |
| `lifetime_warn_after` | `72h` | MR age at which the card's age turns amber |
| `lifetime_error_after` | `120h` | MR age at which it turns red |
| `log.path` | — | Log file. Omit to disable file logging |
| `log.level` | `info` | `debug` \| `info` \| `warn` \| `error` |

## Issue tracker

Optional. Enables the card ticket line with an issue-type icon, the sprint filter, the batch
reviewer editor, and back-link injection.

```yaml
jira:
  instance_url: https://yourorg.atlassian.net
  email: you@example.com
  api_token: your-token        # or $JIRA_TOKEN
  board_id: 42                 # optional; enables the sprint filter
  cache_ttl: 24h
  issue_type_icons:            # optional; overrides the default emoji map
    Bug: "🐛"
    Story: "📖"
    Task: "✅"
    Epic: "⚡"
  remote_link_icon_url: https://…/icon.png   # optional; icon for the created remote link
```

| Key | Default | Meaning |
| --- | --- | --- |
| `instance_url` | — | Tracker base URL |
| `email` | — | Account email, used for Basic auth |
| `api_token` | — | API token; or set `$JIRA_TOKEN` |
| `board_id` | — | Board to read the active sprint from. Required for the sprint filter |
| `cache_ttl` | `24h` | How long issue types are cached on disk |
| `issue_type_icons` | built-in map | Issue type name (case-sensitive) → any single character |
| `remote_link_icon_url` | — | Icon attached to the remote link mrboard creates |

A ticket key is recognised when it appears **in parentheses** in the MR title, e.g.
`feat(OD-2400): …`. Issue types are fetched in the background; a `🎫` placeholder shows while
one is loading or when the type has no icon mapping.

Back-link injection appends a ticket link to the MR description unless the description already
contains the `<!-- mrboard -->` marker, which makes it idempotent. See
[adr/0003-jira-remote-links.md](adr/0003-jira-remote-links.md).

## Notifications

Optional. Enables the `n` key, and also fires automatically when approvers are saved.

```yaml
notifications:
  teams:
    webhook_url: https://outlook.office.com/webhook/...
    user_mappings:             # GitLab username → display name in the message body
      alice: Alice Smith
    user_ids:                  # GitLab username → UPN/email, enables @mention pings
      alice: alice@example.com
```

## External commands

Binds a key on the board to an external program, launched with mrboard's terminal suspended
and resumed on exit. Useful for handing the selected MR to a dedicated review tool that wants
the whole terminal.

```yaml
commands:
  - name: review in tuicr     # shown in the '?' help modal
    key: T
    binary: tuicr-mr          # resolved via $PATH
    args: ["{{.WebURL}}", "{{.ProjectPath}}", "{{.IID}}"]
```

`args` is an argv template with no shell interpretation. Available variables:
`{{.ProjectPath}}`, `{{.IID}}`, `{{.SourceBranch}}`, `{{.TargetBranch}}`, `{{.WebURL}}`,
`{{.Title}}`, `{{.Author}}`.

See [adr/0004-external-command-launcher.md](adr/0004-external-command-launcher.md) for the
design and its non-goals.

## Themes

Five built-in themes: `default`, `dracula`, `nord`, `tokyo-night`, `monokai`. Press `t` for a
live picker; your choice is saved automatically, as is the light/dark/auto mode.

Per-session overrides, which are not saved:

```bash
mrboard --theme dracula
mrboard --mode light
```

Drop any `.json` file into `~/.config/mrboard/themes/` and it appears in the picker; a file
named after a built-in replaces it. Format: [theme-format.md](theme-format.md).

## Troubleshooting

**Authentication failed.** The token needs `api` scope, not `read_api`. Check it hasn't
expired.

**No MRs showing.** Verify the group path or username, then test the API directly:

```bash
curl -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.example.com/api/v4/groups/my-team/merge_requests"
```

**Everything hangs, then errors with `context deadline exceeded`.** Usually the instance is
unreachable rather than slow — most often a VPN that is not connected. Confirm with
`nc -z -w 5 gitlab.example.com 443` before raising `gitlab.timeout`.

**Nothing in the Approved column.** That column means *every designated approver has
approved*. An MR with no designated approvers stays in Needs Review no matter how many plain
reviewers approve it. Use the reviewer editor (`v`, then `space`) to mark approvers.

**Ticket icons not appearing.** Check `jira.instance_url`, `jira.email`, and `jira.api_token`
(or `$JIRA_TOKEN`), then read the log — tracker errors are logged at `warn`.

**Debug logging.**

```yaml
log:
  path: /tmp/mrboard.log
  level: debug
```

For fetch problems specifically, `mrboard --log-level debug fetch` is usually faster than the
TUI: it prints errors straight to stderr instead of behind the loading spinner.

# mrboard

A terminal board for your team's GitLab merge requests — built for daily standups.

## Install

```bash
brew tap ceffo/tap
brew install mrboard
```

## Configuration

Create a config file at `~/.config/mrboard/mrboard.yaml` (or set `$MRBOARD_CONFIG` to point
anywhere you like):

```yaml
gitlab:
  url: https://gitlab.example.com
  token: glpat-xxx        # or set $GITLAB_TOKEN; needs api scope for write operations
  timeout: 30s            # default: 30s

sources:
  - type: group
    ids: [my-team]        # one or more GitLab group paths or numeric IDs

  - type: user
    ids: [alice, bob]     # one or more GitLab usernames

excluded_authors:
  - renovate-bot
  - dependabot

current_user: alice       # your GitLab username — highlights your MRs in the board

log:
  path: /tmp/mrboard.log  # optional; omit to disable file logging
  level: info             # debug | info | warn | error

# Optional: JIRA integration (enables card issue-type icons, sprint filter, and batch editor)
jira:
  instance_url: https://yourorg.atlassian.net
  email: you@example.com
  api_token: your-jira-token   # or set $JIRA_TOKEN
  board_id: 42                 # optional; enables the sprint filter (S key)
  cache_ttl: 24h               # default: 24h
  issue_type_icons:            # optional; override the default emoji map
    Bug: "🐛"
    Story: "📖"
    Task: "✅"
    Epic: "⚡"

# Optional: Teams notifications (enables the n key)
notifications:
  teams:
    webhook_url: https://outlook.office.com/webhook/...
    user_mappings:             # GitLab username → Teams display name
      alice: Alice Smith
      bob: Bob Jones
    user_ids:                  # GitLab username → Teams UPN/email for @mentions
      alice: alice@example.com
      bob: bob@example.com
```

You can mix as many `group` and `user` sources as you need. MRs from all sources are merged
and deduplicated.

## Columns

| Column | What's in it |
| --- | --- |
| **Draft** | MRs marked as draft |
| **Needs Review** | Waiting for reviewer feedback |
| **Needs Author Action** | Reviewer left comments; author needs to respond |
| **Approved** | GitLab reports `detailed_merge_status == mergeable` (all approvals, CI, and branch protection satisfied) |

## Keybindings

### Board

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Navigate cards |
| `←`/`h`, `→`/`l` | Switch columns |
| `↵` | Open detail pane |
| `o` | Open MR in browser |
| `r` | Refresh |
| `s` | Cycle sort order |
| `S` | Toggle sprint filter (requires `jira.board_id`) |
| `tab` | Toggle "my view" (MRs relevant to you) |
| `v` | Open reviewer editor |
| `E` | Open batch reviewer editor (groups sibling MRs by JIRA ticket) |
| `d` | Open diff view |
| `J` | Open linked JIRA ticket in browser |
| `n` | Send Teams notification for focused card |
| `t` | Open theme picker |
| `,` | Open settings |
| `q` / `ctrl+c` | Quit |

### Reviewer editor

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Navigate |
| `space` | Toggle approver flag |
| `d` | Remove reviewer |
| `/` | Search members |
| `T` | Set team |
| `↵` | Save |
| `v` / `esc` | Cancel |

### Batch reviewer editor

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Navigate |
| `tab` | Switch between MR list and reviewer panels |
| `space` | Toggle approver flag |
| `d` | Remove reviewer |
| `↵` | Preview changes |
| `E` / `esc` | Cancel |

**Batch preview screen**

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Navigate rows |
| `space` | Include / exclude a row |
| `↵` | Apply (writes only rows with detected changes) |
| `esc` | Back to editor |

### Diff view

| Key | Action |
| --- | --- |
| `p` / `n` | Previous / next file |
| `↑`/`k`, `↓`/`j` | Scroll |
| `ctrl+u`, `ctrl+d` | Half-page up / down |
| `g` / `G` | Jump to top / bottom |
| `o` | Open file in browser |
| `d` / `esc` | Close |
| `q` | Quit |

### Detail pane

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Scroll |
| `o` | Open MR in browser |
| `esc` / `↵` | Close |
| `q` | Quit |

### Settings panel

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Navigate items |
| `←`/`h`, `→`/`l` | Switch sections |
| `tab` / `shift+tab` | Next / previous tab |
| `space` | Toggle option |
| `↵` | Apply |
| `,` / `esc` | Close |

## Theming

Five built-in themes: `default`, `dracula`, `nord`, `tokyo-night`, `monokai`.

Press **`t`** to open the live theme picker — the board stays visible behind it so you see
changes in real time. You can also switch between **auto** (follows your terminal's background),
**dark**, and **light** mode from within the picker. Your selection is saved automatically.

To override for a single session without saving:

```bash
mrboard --theme dracula
mrboard --mode light
mrboard --theme nord --mode dark
```

**Custom themes:** drop any `.json` file into `~/.config/mrboard/themes/` and it appears in
the picker automatically. A file with the same name as a built-in overrides it. See
[docs/theme-format.md](docs/theme-format.md) for the format.

## JIRA integration

When `jira` is configured, each card shows a dedicated third line with an issue-type icon and
the JIRA key (e.g. `🐛 OD-3345`). Icons are fetched asynchronously — a `🎫` placeholder
appears while loading — and results are cached to disk for `cache_ttl` (default 24 hours) to
minimise API traffic.

**Sprint filter (`S` key):** when `jira.board_id` is set, pressing `S` restricts the board to
MRs whose linked JIRA issue is part of the active sprint. A sprint indicator appears in the board
header while the filter is on.

**JIRA backlink injection:** when fetching MRs, mrboard automatically appends a JIRA link to the
MR description if the `<!-- mrboard -->` marker is absent, keeping GitLab MR descriptions in sync
with their linked tickets. This runs in the background and does not block the board.

**Batch reviewer editor (`E` key):** opens a full-screen editor pre-filled from the focused
card's reviewers, with a panel listing all sibling MRs that share the same JIRA ticket. A preview
diff screen shows per-MR reviewer changes before committing. Writes are skipped for MRs where
nothing changed, making the operation idempotent.

Issue type icons can be customised via `jira.issue_type_icons` in the config — the key is the
JIRA issue type name (case-sensitive) and the value is any single emoji or character.

## Teams notifications

When `notifications.teams.webhook_url` is set, pressing **`n`** on a focused card fires a webhook
to post a notification about MR review assignment. The card includes the MR title, author, and
reviewer list.

`user_mappings` translates GitLab usernames to Teams display names in the notification body.
`user_ids` maps GitLab usernames to Teams UPNs (email addresses) to enable `@mention` pings in
the Adaptive Card.

Approver saves also fire a Teams notification automatically when a webhook is configured.

## Troubleshooting

**Authentication failed**

- Make sure your token has `api` scope (`read_api` is not sufficient — the reviewer editor writes
  back to GitLab)
- Check it hasn't expired: `echo $GITLAB_TOKEN`

**No MRs showing**

- Verify the group ID or username is correct
- Test the API directly:

```bash
curl -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.example.com/api/v4/groups/my-team/merge_requests"
```

**Slow or timing out**

```bash
MRBOARD_TIMEOUT=60s mrboard
```

**JIRA icons not appearing**

- Check that `jira.instance_url`, `jira.email`, and `jira.api_token` (or `$JIRA_TOKEN`) are set
- Enable debug logging and inspect the log file — JIRA fetch errors are logged at `warn` level

**Debug logging**

Add to your config:

```yaml
log:
  path: /tmp/mrboard.log
  level: debug
```

Then: `cat /tmp/mrboard.log`

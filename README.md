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
  token: glpat-xxx        # or set $GITLAB_TOKEN env var
  required_approvals: 2   # default: 2
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
```

You can mix as many `group` and `user` sources as you need. MRs from all sources are merged
and deduplicated.

## Columns

| Column | What's in it |
| --- | --- |
| **Draft** | MRs marked as draft |
| **Needs Review** | Waiting for reviewer feedback |
| **Needs Author Action** | Reviewer left comments; author needs to respond |
| **Ready to Merge** | Has the required number of approvals |

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

## Troubleshooting

**Authentication failed**

- Make sure your token has `api` and `read_api` scopes
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

**Debug logging**

Add to your config:

```yaml
log:
  path: /tmp/mrboard.log
  level: debug
```

Then: `cat /tmp/mrboard.log`

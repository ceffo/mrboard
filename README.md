# mrboard

A terminal-based GitLab merge request board — displays your team's MRs organized by review phase.

## Quick Start

Create a config file at `~/.config/mrboard/mrboard.yaml`:

```yaml
gitlab:
  url: https://gitlab.example.com
  token: glpat-xxx              # or set $GITLAB_TOKEN env var
  required_approvals: 2         # default: 2

sources:
  - type: group
    id: my-team

  - type: user
    username: alice

excluded_authors:
  - renovate-bot
  - dependabot
```

Then run:

```bash
mrboard
```

## Configuration

The config file is searched in this order:

- `$MRBOARD_CONFIG` (environment variable)
- `~/.config/mrboard/mrboard.yaml`
- `./mrboard.yaml` (current directory)

### Required Fields

- **`gitlab.url`** — Your GitLab instance URL
- **`gitlab.token`** — GitLab personal access token (or set `$GITLAB_TOKEN`)
- **`sources`** — Where to fetch MRs from:
  - `type: group` + `id: <group-id>` — all MRs in a GitLab group
  - `type: user` + `username: <username>` — all MRs authored by a user

### Optional Fields

- **`gitlab.required_approvals`** — Approval threshold (default: 2)
- **`excluded_authors`** — List of usernames to ignore (e.g., bots)

## Keyboard Shortcuts

- **↑/k, ↓/j, ←/h, →/l** — Navigate cards
- **Enter** — View MR details
- **Esc** — Close details
- **o** — Open MR in GitLab
- **r** — Refresh
- **s** — Sort
- **tab** — Toggle view
- **f** — Filter
- **t** — Open theme picker
- **q** — Quit

## Theming

mrboard ships five built-in themes: `default`, `dracula`, `nord`, `tokyo-night`, `monokai`.

### Live theme picker

Press **`t`** to open the theme picker overlay. The board stays visible behind it so you see changes in real time as you navigate:

- **↑/↓** — Select theme or mode
- **Tab** — Switch between theme list and mode pane
- **t / Esc** — Close

The selected theme and mode are saved automatically on every navigation move — no confirmation step needed.

### Mode

The right pane of the picker lets you choose:

- **auto** (default) — follows your terminal's background colour
- **dark** — force dark palette
- **light** — force light palette

### CLI flags (session-only, not persisted)

```bash
mrboard --theme dracula          # override theme for this session
mrboard --mode light             # override mode for this session
mrboard --theme nord --mode dark
```

### Custom themes

Drop any `.json` file into `~/.config/mrboard/themes/` and it appears in the picker automatically. Custom themes use the same format as built-in themes. A custom file with the same name as a built-in overrides it.

## Columns

- **Draft** — MRs in draft mode
- **Needs Review** — Waiting for reviewer feedback
- **Needs Author Action** — Reviewer comments; awaiting author response
- **Ready to Merge** — Meets approval threshold

## Environment Variables

- `GITLAB_TOKEN` — Override config token
- `MRBOARD_CONFIG` — Config file path
- `MRBOARD_TIMEOUT` — HTTP timeout (default: 30s)
- `MRBOARD_DEBUG` — Debug log file path

## Commands

```bash
mrboard                               # Launch TUI
mrboard fetch                         # Export MRs as JSON
mrboard fetch --debug /tmp/debug.log  # Save debug info
```

## Troubleshooting

**Config file not found:**

```bash
echo $MRBOARD_CONFIG
cat ~/.config/mrboard/mrboard.yaml
cat ./mrboard.yaml
```

**Authentication failed:**

- Verify `GITLAB_TOKEN` is set and not expired
- Check token has `api` and `read_api` scopes

**No MRs showing:**

- Verify group IDs and usernames exist
- Test the API manually:

```bash
curl -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.example.com/api/v4/groups/my-team/merge_requests"
```

**Slow or timeout:**

```bash
MRBOARD_TIMEOUT=60s mrboard
```

Enable debug logging:

```bash
MRBOARD_DEBUG=/tmp/mrboard.log mrboard
cat /tmp/mrboard.log
```

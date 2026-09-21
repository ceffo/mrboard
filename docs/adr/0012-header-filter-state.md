# ADR-0012: Filter State Belongs in the Header

**Status**: Accepted

## Context

The board filters MRs on six independent dimensions: the view scope (`tab`, "my MRs only"), the
sprint scope (`S`), the four phase columns, and three exclusion lists — assignee, reviewer, and
issue ID. The last four are set from the Filters tab and persist in `state.yaml`, so a filter
outlives the session that set it.

Before this decision the board reported that state through three unrelated mechanisms, and
reported it badly:

- `[filtered]` was a boolean OR across every dimension. One bit, ten columns, and it could not
  distinguish "one phase hidden" from "half the team excluded three days ago".
- `Model.isFilterActive()` included `sprintFilterActive`, so turning the sprint filter on lit
  **both** `[filtered]` and `[sprint]` — two tags for one filter.
- The view scope appeared as a `— @username` suffix appended to the app title, which put state
  into the identity zone and printed a person's name in the chrome. It also collided with the
  diff view, which borrows the same `SetTitle` field for its own caption.

The root cause was structural rather than cosmetic: `headerWidget` stored *presentation*
(`title string`), not *state*. There was no field for "the board is scoped to me", so it had to
be smuggled into the title. Anything that fixed the display without giving the header somewhere
to keep filter state would have grown a fourth mechanism rather than replacing the three.

The Filters tab work on the same branch made this worse by making the exclusion lists genuinely
useful — the richer the filtering, the less adequate a single "something is on" bit becomes.

## Non-goals

- **The header never names an individual.** No usernames, no ticket keys, no "included/excluded"
  prose. The bar's job is to make the user *know to open the Filters tab*, not to replace it.
  Leaking a teammate's name into permanent chrome is what the `— @username` suffix did wrong.
- **Not a second control surface.** The bar is read-only. Filters are changed from the Filters
  tab and the `tab`/`S` keys, and nowhere else.
- **No new theme color tokens.** Every style resolves from the existing `ColorKey` set, so a
  third-party theme keeps working without being republished.
- **The fetch scope stays out.** `include_reviewer_mrs` decides what is fetched, not what is
  shown; it grows the population rather than trimming it, so it cannot be expressed in the bar's
  arithmetic (see the invariant below). It lives on the General tab only.

## Decision

The header has three zones, each with exactly one job:

| Zone | Owns | Rule |
|---|---|---|
| left | every active filter, and the MR count they produce | grows and shrinks with filter state |
| centre | app or view identity | never encodes state; first thing dropped |
| right | snapshot age, refresh spinner, sort mode | never mentions filters |

All filter feedback is therefore left-anchored, in a single background-tinted strip rather than
N discrete chips. Discrete chips would each need their own padding and gap — roughly eighteen
extra columns at full filter load, which is precisely the width that decides whether the last
segment survives on an 80-column terminal — and their bracket shape would collide with the card
reviewer pills (`[@abarde ⏳ 11d 2h]`), where brackets already mean "a person on this MR".

### Segment vocabulary

Two shapes, one rule each:

- A **scope** is a bare lowercase word in `Accent`: `me`, `sprint`. These are modes the user
  stands in and toggles constantly during a standup, so they read as a current stance rather
  than as a warning.
- A **reduction** is `tag×n` in `Warning`: `col×2`, `asg×3`, `rev×2`, `tkt×5`. The count is of
  values *hidden*, because the failure mode these dimensions cause is data being withheld
  silently — someone un-ticked a teammate three days ago and the board has been lying since.

### The count

The count leads the bar and is never dropped. It has two forms: `26 mrs` on the plain header
background when nothing is filtered, and `6/26 mrs` on the tinted strip when anything is.

**The slash is the "filtering is on" signal.** That is the entire information content of the old
`[filtered]` tag, delivered inside a number the user was already reading, which is why the tag
could be deleted rather than restyled. When nothing is filtered there is no slash, no tint, and
no bar — the quiet state is genuinely quiet.

### The denominator invariant

The denominator is the population the filters actually ran against (`src` in
`Model.applyMRFilter`), **not** every fetched MR. `applyMRFilter` prunes reviewer-sourced MRs
before filtering when `include_reviewer_mrs` is off; counting against `allMRs` would leave part
of the gap between numerator and denominator with no segment explaining it.

The invariant is: **every segment on the bar accounts for part of why `shown < total`.** It is
what makes the bar readable left-to-right as "6 of 26, because: me, sprint, asg×3", and it is
the reason the fetch scope is a non-goal above rather than a seventh segment.

### Degradation

Pieces are dropped **whole** as the terminal narrows — never truncated mid-segment — in this
order, mirroring the policy `footerWidget.fitItems` already uses for the other chrome line:

```
title → sort → tkt → rev → col → asg → age → sprint → me
```

Reductions outrank the title and the sort mode because a forgotten filter misrepresents the
board while a missing app name costs nothing. Among the reductions, hidden *people* survive
longest: they are the exclusions most likely to hide someone's work at a standup. Scopes survive
longest of all, since they change what every other number on the line means.

Anything dropped from the bar is counted by a trailing `+n`, so "there is more than you can see"
— the one thing `[filtered]` did well — survives, quantified and present only when true. The
refresh spinner outlives the age label it belongs to: a fetch in flight stays visible however
narrow the terminal.

Note that `col×n` is **not** redundant with the board itself. `boardWidget` holds a fixed
`[4]columnWidget`, so all four columns always render; a filtered-out phase shows as `Draft (0)`,
which is indistinguishable from "there are no drafts today". That is why it sits mid-ladder
rather than being dropped first.

### Layout and overlays

The title is centred on the full terminal width and then **clamped** rightwards off the bar,
rather than re-centred in the space the bar leaves. Clamping holds it still until the bar
actually reaches it; re-centring would nudge the title on every filter keystroke.

When an overlay borrows the header for its own caption — the diff view sets both a title and a
stats override — the filter bar is suppressed (`headerWidget.borrowed()`). Its counts describe a
board that is no longer on screen.

### Shape in code

`headerFilterState` (two bools, four counts) is the whole contract between the model and the
header. `Model.headerFilterState()` derives it from `m.filter`, `m.viewMode` and
`m.sprintFilterActive`; `headerWidget.SetFilterState(total, state)` feeds it, matching the
existing `Set*` pattern. The widget does no domain logic and the model does no layout.

Six `Styles` fields carry it — `HeaderFilterBar`, `HeaderFilterScope`, `HeaderFilterReduce`,
`HeaderFilterSep`, `HeaderCountShown`, `HeaderCountTotal` — all from existing tokens.
`FilterActive` lost both call sites and was deleted, as were `Model.isFilterActive()`,
`SetFilterActive`, `SetSprintFilterActive`, and the `— @username` title suffix.

## Consequences

- **`Total:N` is gone from the right side.** The count moved left because it is the *consequence*
  of the filters. Anything that wants a post-filter count reads the bar.
- **Reduction counts describe the filter, not the data.** `len(ExcludedAssignees)` includes
  usernames no longer present in any fetched MR, so a filter excluding someone who left the team
  still announces itself as `asg×1` instead of silently reading zero. This is deliberate: a
  filter you cannot see is the problem being solved.
- **`SetTitle` must never carry state again.** It is now identity-only and is the first thing
  dropped under width pressure. A future view name may use it; a future mode indicator may not.
- **Adding a seventh filter dimension means adding a `headerPiece`**, choosing its rank in the
  drop ladder, and a segment in `filterSegments()`. The ladder is a total order by design —
  there is no "equal priority" tier to fall into, because under width pressure something has to
  go first.
- **A theme whose `BgElevated` matches `BgBase` loses the strip shape**, not the information:
  every segment stays legible as coloured text with `·` separators. Nothing depends on the
  background being visible.
- The bar is wider than the tags it replaced, so on a terminal under roughly 64 columns the app
  name is usually absent. That trade is intentional — at that width you would rather know what
  the board is hiding than read the word `mrboard`.

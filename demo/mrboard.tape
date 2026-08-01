# Recording script for the README GIF. Regenerate with `just demo`.
#
# Keep the whole run under ~55s. Card ages render truncated to the minute and
# the fixture's offsets are whole minutes, so the first age flip happens 60s
# after launch — staying under that keeps every frame's text reproducible.
#
# Deliberately never presses `o` or `J`: both shell out to the system browser,
# which would pop a window over the recording.

Output demo/mrboard.gif

Set Shell zsh
Set FontSize 13
Set Width 1560
Set Height 860
Set Padding 14
Set Framerate 20
Set WindowBar Colorful
Set Margin 12
Set MarginFill "#1a1b26"
Set BorderRadius 8

# Boot off-camera: the demo board is warm from its seeded cache, so it is
# already interactive when the recording starts.
Hide
Type "./scripts/demo-tui.sh" Enter
Sleep 4s
Show

# The board: four columns, every reviewer state, ages from minutes to months.
Sleep 3s

# Needs Review, second card (!418) -> the reviewer/approver editor.
#
# NOTE: moving between columns keeps the current ROW, so each Right below lands
# on the same row in the next column. That is why there is no extra Down after
# the first one — adding any overshoots onto a card with nothing to show.
Right
Sleep 1s
Down
Sleep 1500ms
Type "v"
Sleep 3s
Escape
Sleep 1s

# Needs Author Action, same row (!410) -> detail pane, with threads.
Right
Sleep 1s
Enter
Sleep 4s
Escape
Sleep 800ms

# Approved -> the multi-file diff on !415. Row 2 clamps to !96 (a one-file
# diff), so step Up to the card with the interesting one.
Right
Sleep 800ms
Up
Sleep 1s
Type "d"
Sleep 3s
Type "n"
Sleep 2500ms
Escape
Sleep 1s

# Sprint filter narrows the board to the active sprint, then back.
Type "S"
Sleep 2500ms
Type "S"
Sleep 1500ms

# Settings, then the contextual help modal.
Type ","
Sleep 3s
Escape
Sleep 800ms
Type "?"
Sleep 3s
Escape
Sleep 1s

Type "q"
Sleep 1s

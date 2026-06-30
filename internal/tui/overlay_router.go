package tui

// overlayKind identifies which exclusive overlay currently owns key input and
// rendering focus. overlayKindNone means the board (and optional detail panel)
// owns focus.
type overlayKind int

const (
	overlayKindNone                overlayKind = iota
	overlayKindDiffView                        // full-screen diff view
	overlayKindSettings                        // settings popup
	overlayKindReviewerEditor                  // reviewer editor popup
	overlayKindBatchReviewerEditor             // batch reviewer editor popup
	overlayKindBatchPreview                    // batch preview screen (confirm selection before apply)
)

// overlayRouter is a single-active-overlay state machine. Only one exclusive
// overlay (diff view, settings, reviewer editor) can own focus at a time.
// The detail side-panel is not tracked here — it can remain visible while an
// overlay is active (e.g. detail open → user opens diff view on top).
type overlayRouter struct {
	kind overlayKind
}

func (r *overlayRouter) openOverlay(k overlayKind) { r.kind = k }
func (r *overlayRouter) closeOverlay()             { r.kind = overlayKindNone }
func (r overlayRouter) active() overlayKind        { return r.kind }
func (r overlayRouter) isDiffView() bool           { return r.kind == overlayKindDiffView }

package tui

import (
	"fmt"
	"reflect"
	"sort"

	"charm.land/bubbles/v2/key"
)

// Priority ranks an Action for inclusion in the footer. Lower values are
// filled into the status line first; PriorityModal actions never appear there
// and are only listed in the '?' help modal.
type Priority int

// Footer priorities, from never-dropped to modal-only.
const (
	PriorityPinned Priority = iota // survives the narrowest terminals
	PriorityCore                   // primary interactions (navigation, open/close)
	PriorityCommon                 // frequent actions
	PriorityModal                  // help modal only, never in the footer
)

// Category groups actions inside the '?' help modal.
type Category int

// Help-modal categories; the modal renders them in this order.
const (
	CategoryNavigate Category = iota
	CategoryAct
	CategoryView
	CategoryGeneral
)

// categoryOrder fixes the column order of the help modal.
var categoryOrder = []Category{CategoryNavigate, CategoryAct, CategoryView, CategoryGeneral}

func (c Category) String() string {
	switch c {
	case CategoryNavigate:
		return "Navigate"
	case CategoryAct:
		return "Act"
	case CategoryView:
		return "View"
	case CategoryGeneral:
		return "General"
	default:
		return "Other"
	}
}

// Action is the single definition of one user-invokable command: its keys,
// its static help label, its footer priority, and its help-modal category.
// Dispatch, footer, and help modal all consume the same value.
type Action struct {
	key.Binding
	Priority Priority
	Category Category
}

// Match reports whether the pressed key triggers this action.
func (a Action) Match(k fmt.Stringer) bool { return key.Matches(k, a.Binding) }

// Act builds an Action. helpKey is the display form ("↑/k"), label the static
// verb ("scroll up" — never state), keys the raw bubbletea key names. When
// keys is empty, helpKey doubles as the single raw key.
func Act(helpKey, label string, pri Priority, cat Category, keys ...string) Action {
	if len(keys) == 0 {
		keys = []string{helpKey}
	}
	return Action{
		Binding:  key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, label)),
		Priority: pri,
		Category: cat,
	}
}

// footerGroup collapses several actions into one status-line item (e.g. four
// arrow bindings into "↑↓←→ move"). The help modal still lists members
// individually.
type footerGroup struct {
	helpKey string
	label   string
	members []*Action
}

// Context is a named set of actions that owns key input while active.
// Contexts are stacked (base at the bottom, focused overlay on top); a key
// bound higher in the stack shadows the same key lower down.
type Context struct {
	name         string
	title        string
	capturesText bool // a focused text input consumes printable keys
	actions      []*Action
	groups       []footerGroup
	byKey        map[string]string // raw key → field name, for conflict checks
}

// ContextOpt customises a Context at construction time.
type ContextOpt func(*Context)

// WithCapturesText marks the context as owning a focused text input: printable
// keys (including '?' and 'q') must reach the input, so base-context actions
// are suppressed while it is on top of the stack.
func WithCapturesText() ContextOpt {
	return func(c *Context) { c.capturesText = true }
}

// WithFooterGroup renders the given member actions as a single footer item.
func WithFooterGroup(helpKey, label string, members ...*Action) ContextOpt {
	return func(c *Context) {
		c.groups = append(c.groups, footerGroup{helpKey: helpKey, label: label, members: members})
	}
}

// allContexts is the registry populated by NewContext; the conflict test
// walks it so every context defined in keys.go is checked automatically.
var allContexts []*Context

// AllContexts returns every registered context.
func AllContexts() []*Context { return allContexts }

// NewContext registers a context. keymap must be a pointer to a struct whose
// Action fields define the context's bindings; they are enumerated by
// reflection in declaration order, so adding a field is the only step needed
// to register a new binding. Panics if two actions within the context claim
// the same key — intra-context conflicts are always bugs (cross-context
// shadowing is legal: the higher context wins).
func NewContext(name, title string, keymap any, opts ...ContextOpt) *Context {
	v := reflect.ValueOf(keymap)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("keymap %q: NewContext requires a pointer to a struct", name))
	}
	c := &Context{name: name, title: title, byKey: map[string]string{}}
	elem := v.Elem()
	actionType := reflect.TypeOf(Action{})
	for i := range elem.NumField() {
		if elem.Field(i).Type() != actionType {
			panic(fmt.Sprintf("keymap %q: field %s is not an Action", name, elem.Type().Field(i).Name))
		}
		a, ok := elem.Field(i).Addr().Interface().(*Action)
		if !ok {
			panic(fmt.Sprintf("keymap %q: field %s is not addressable", name, elem.Type().Field(i).Name))
		}
		fieldName := elem.Type().Field(i).Name
		for _, k := range a.Keys() {
			if prev, dup := c.byKey[k]; dup {
				panic(fmt.Sprintf("keymap %q: key %q bound to both %s and %s", name, k, prev, fieldName))
			}
			c.byKey[k] = fieldName
		}
		c.actions = append(c.actions, a)
	}
	for _, o := range opts {
		o(c)
	}
	allContexts = append(allContexts, c)
	return c
}

// NewDynamicContext builds a context from an explicit slice of actions rather
// than reflecting over a keymap struct's fields — for action sets whose count
// and keys are only known at runtime (e.g. user-configured commands, see
// docs/adr/0004-external-command-launcher.md). It builds the same byKey map as
// NewContext, so footerItems, helpSections, and cross-context shadowing all
// work unchanged; unlike NewContext it is not added to allContexts, since that
// registry is for the fixed, compile-time set of contexts TestNoKeyConflicts
// walks, not ones rebuilt per config load. Panics on an intra-context key
// collision, the same invariant NewContext enforces — configured-command key
// collisions are expected to be rejected earlier, at config load time.
func NewDynamicContext(name, title string, actions []*Action, opts ...ContextOpt) *Context {
	c := &Context{name: name, title: title, byKey: map[string]string{}}
	for _, a := range actions {
		label := a.Help().Desc
		for _, k := range a.Keys() {
			if prev, dup := c.byKey[k]; dup {
				panic(fmt.Sprintf("keymap %q: key %q bound to both %s and %s", name, k, prev, label))
			}
			c.byKey[k] = label
		}
		c.actions = append(c.actions, a)
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name returns the context's slug identifier.
func (c *Context) Name() string { return c.name }

// Title returns the display title used in the help modal header.
func (c *Context) Title() string { return c.title }

// CapturesText reports whether a focused text input owns printable keys.
func (c *Context) CapturesText() bool { return c.capturesText }

// binds reports whether the context claims the given raw key (used for
// shadowing checks against lower stack entries).
func (c *Context) binds(rawKey string) bool {
	_, ok := c.byKey[rawKey]
	return ok
}

// footerItem is one candidate entry for the status line.
type footerItem struct {
	key      string
	label    string
	priority Priority
}

// footerItems returns the context's footer candidates sorted by
// (priority, declaration order), with grouped actions collapsed into their
// group entry and modal-only or disabled actions excluded.
func (c *Context) footerItems() []footerItem {
	groupOf := map[*Action]*footerGroup{}
	groupHead := map[*footerGroup]*Action{}
	for gi := range c.groups {
		g := &c.groups[gi]
		for _, m := range g.members {
			groupOf[m] = g
		}
	}
	// The group renders at the position of its first-declared enabled member.
	for _, a := range c.actions {
		if g, ok := groupOf[a]; ok && groupHead[g] == nil && a.Enabled() {
			groupHead[g] = a
		}
	}

	var items []footerItem
	for _, a := range c.actions {
		if !a.Enabled() {
			continue
		}
		if g, ok := groupOf[a]; ok {
			if groupHead[g] == a {
				items = append(items, footerItem{key: g.helpKey, label: g.label, priority: a.Priority})
			}
			continue
		}
		if a.Priority == PriorityModal {
			continue
		}
		items = append(items, footerItem{key: a.Help().Key, label: a.Help().Desc, priority: a.Priority})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].priority < items[j].priority })
	return items
}

// helpEntry is one key/label row of the help modal.
type helpEntry struct {
	key   string
	label string
}

// helpSection is one category column of the help modal.
type helpSection struct {
	title   string
	entries []helpEntry
}

// helpSections builds the '?' modal content for a context stack
// (bottom → top). Actions lower in the stack are hidden when a higher context
// claims any of their keys (shadowing) or when the top context captures text.
func helpSections(stack []*Context) []helpSection {
	byCat := map[Category][]helpEntry{}
	claimed := map[string]bool{}
	for i := len(stack) - 1; i >= 0; i-- {
		ctx := stack[i]
		if i < len(stack)-1 && stack[len(stack)-1].CapturesText() {
			break // text input on top: lower contexts are unreachable
		}
		for _, a := range ctx.actions {
			if !a.Enabled() {
				continue
			}
			shadowed := false
			for _, k := range a.Keys() {
				if claimed[k] {
					shadowed = true
					break
				}
			}
			if shadowed {
				continue
			}
			byCat[a.Category] = append(byCat[a.Category], helpEntry{key: a.Help().Key, label: a.Help().Desc})
		}
		for k := range ctx.byKey {
			claimed[k] = true
		}
	}

	var sections []helpSection
	for _, cat := range categoryOrder {
		if entries := byCat[cat]; len(entries) > 0 {
			sections = append(sections, helpSection{title: cat.String(), entries: entries})
		}
	}
	return sections
}

# Custom Theme Format

Custom themes live in `~/.config/mrboard/themes/`. Any `.json` file dropped there appears
in the theme picker automatically. A file whose name matches a built-in theme (e.g.
`dracula.json`) overrides it.

## Structure

Each theme file defines a `styles` object with named color tokens. Each token has a `"dark"`
value and an optional `"light"` value. If `"light"` is omitted, the `"dark"` value is used
in both modes.

Color values can be either a **hex string** (`"#bd93f9"`) or a **terminal 256-color index**
as a string (`"99"`, `"235"`).

### Minimal example (dark-only)

```json
{
  "styles": {
    "bg-base":      { "dark": "#1e1e2e" },
    "fg-high":      { "dark": "#cdd6f4" },
    "accent":       { "dark": "#cba6f7" },
    "border-focus": { "dark": "#cba6f7" }
  }
}
```

Any token you omit falls back to the built-in default theme's value for that token.

### Full example (dark + light)

```json
{
  "styles": {
    "bg-base":      { "dark": "#1e1e2e", "light": "#eff1f5" },
    "bg-elevated":  { "dark": "#313244", "light": "#e6e9ef" },
    "bg-overlay":   { "dark": "#45475a", "light": "#dce0e8" },
    "fg-high":      { "dark": "#cdd6f4", "light": "#4c4f69" },
    "fg-medium":    { "dark": "#bac2de", "light": "#5c5f77" },
    "fg-low":       { "dark": "#6c7086", "light": "#9ca0b0" },
    "border":       { "dark": "#585b70", "light": "#bcc0cc" },
    "border-focus": { "dark": "#cba6f7", "light": "#8839ef" },
    "accent":       { "dark": "#cba6f7", "light": "#8839ef" },
    "success":      { "dark": "#a6e3a1", "light": "#40a02b" },
    "warning":      { "dark": "#fab387", "light": "#fe640b" },
    "danger":       { "dark": "#f38ba8", "light": "#d20f39" },
    "info":         { "dark": "#89dceb", "light": "#04a5e5" }
  }
}
```

## Token reference

| Token | Used for |
| --- | --- |
| `bg-base` | Main background |
| `bg-elevated` | Card and panel backgrounds |
| `bg-overlay` | Popup and overlay backgrounds |
| `fg-high` | Primary text (high contrast) |
| `fg-medium` | Secondary text |
| `fg-low` | Muted / subtle text |
| `border` | Default border |
| `border-focus` | Focused or active border |
| `accent` | Selected items and emphasis |
| `success` | Approved / ready state |
| `warning` | Pending / needs attention state |
| `danger` | Blocked / rejected state |
| `info` | Informational / neutral highlights |

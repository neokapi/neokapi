# Theming

This document describes how the neokapi UI theme system works and how to change the theme across all framework and platform applications.

## Architecture

The theme is defined as CSS custom properties (oklch color space) in two files that must be kept in sync:

| File | Scope |
|------|-------|
| `packages/ui/src/styles/globals.css` | Shared framework theme — imported by all platform apps (bowrain web, bowrain desktop, ctrl, pulse, keycloak) |
| `framework/apps/kapi-desktop/frontend/src/index.css` | Kapi Desktop — standalone copy (not imported, defines same variables) |

The platform copy at `platform/packages/ui/src/styles/globals.css` is identical to `packages/ui/src/styles/globals.css` and must be updated together.

### How apps inherit the theme

Platform apps (bowrain, web, ctrl, pulse) import the shared theme:
```css
@import "@neokapi/ui/styles/globals.css";
```

Kapi Desktop defines its own variables (identical values) because it doesn't import from `platform/packages/ui` and needs to add kapi-specific extras (animations, pill lightness variables).

## Current theme: Sandstone

Source: https://tweakcn.com/themes/cmmi1ovml000404jlb6e44j42

Warm, earthy palette with gold/amber accents. Key characteristics:
- **Primary**: warm near-black (`oklch(0.2931 0 0)`) in light, warm off-white in dark
- **Accent**: gold (`oklch(0.8778 0.1383 86.1920)`)
- **Surfaces**: sandy warm tint (hue ~106)
- **Font**: DM Sans / DM Mono
- **Radius**: 0.375rem (6px)
- **Letter spacing**: -0.025em (tighter body text)

## How to change the theme

### Step 1: Choose a theme

Browse themes at https://tweakcn.com. Each theme has a registry URL like:
```
https://tweakcn.com/r/themes/<theme-id>
```

Fetch the JSON to get the exact oklch values for light and dark modes.

### Step 2: Update the CSS files

Update these three files with the new variable values:

1. **`packages/ui/src/styles/globals.css`** — primary source of truth
2. **`platform/packages/ui/src/styles/globals.css`** — identical copy
3. **`framework/apps/kapi-desktop/frontend/src/index.css`** — kapi desktop copy

Each file has `:root { }` (light) and `.dark { }` (dark) blocks with the same variable names:

```css
:root {
  --background: oklch(...);
  --foreground: oklch(...);
  --primary: oklch(...);
  /* ... all other variables */
}

.dark {
  --background: oklch(...);
  /* ... dark variants */
}
```

### Step 3: Update the `@theme inline` block

The `@theme inline` block maps CSS variables to Tailwind utility names. Update:
- `--font-sans` and `--font-mono` if the theme specifies different fonts
- `--radius` base value
- `--tracking-normal` for letter spacing

### Step 4: Verify

```bash
# Build all frontends
cd framework/apps/kapi-desktop/frontend && npx vite build
cd framework/apps/kapi-desktop/frontend && npm run storybook:build
cd framework/apps/kapi-desktop/frontend && npm run test -- --run

# Also verify platform apps if globals.css changed:
cd platform/apps/bowrain/frontend && npm run build
cd platform/apps/web && npm run build
```

## Flow editor theming

The flow editor (`packages/flow-editor/`) uses CSS variable references via `packages/flow-editor/src/theme.ts` instead of hardcoded colors. It maps semantic names to `var(--background)`, `var(--border)`, etc.

**Tool category colors** (translate, validate, transform, convert, enrich, pipeline) are intentionally distinct per-category and defined in `packages/flow-editor/src/category.ts`. They use fixed oklch hues that complement the overall theme but are not theme variables — changing them requires editing category.ts directly.

### Theme token mapping (theme.ts)

| Token | CSS Variable | Usage |
|-------|-------------|-------|
| `theme.bg` | `var(--background)` | Panel/canvas backgrounds |
| `theme.bgCard` | `var(--card)` | Node backgrounds, input fields |
| `theme.bgMuted` | `var(--muted)` | Toggle off-state |
| `theme.bgSecondary` | `var(--secondary)` | Group headers, hover states |
| `theme.fg` | `var(--foreground)` | Primary text |
| `theme.fgMuted` | `var(--muted-foreground)` | Secondary text, icons |
| `theme.fgSecondary` | `var(--secondary-foreground)` | Field labels |
| `theme.border` | `var(--border)` | All borders |
| `theme.accent` | `var(--accent)` | Run button, toggle on-state |
| `theme.accentFg` | `var(--accent-foreground)` | Text on accent |
| `theme.primary` | `var(--primary)` | Primary actions |
| `theme.primaryFg` | `var(--primary-foreground)` | Text on primary |
| `theme.destructive` | `var(--destructive)` | Remove button |

## Resource browser dark mode

The resource browser components (LocalePill, TermStatusBadge) use dynamic oklch colors with lightness values from CSS custom properties:

```css
:root {
  --pill-bg-l: 0.92;   /* light bg for locale pills */
  --pill-fg-l: 0.4;    /* dark text for locale pills */
  --badge-bg-l: 0.92;
  --badge-fg-l: 0.4;
}
.dark {
  --pill-bg-l: 0.25;   /* dark bg */
  --pill-fg-l: 0.75;   /* light text */
  --badge-bg-l: 0.25;
  --badge-fg-l: 0.75;
}
```

These are defined in `framework/apps/kapi-desktop/frontend/src/index.css` only (not in the shared globals).

---
sidebar_position: 9
title: "Theme System"
---
# Theme System

This note documents the theme system used across all four frontend projects. The design system builds on `shadcn-glass-ui` and Tailwind CSS v4, with an OKLCH-based token hierarchy and two active themes: **Aurora** (dark) and **Light**.

## shadcn-glass-ui Integration

The shared UI package (`packages/ui/`) uses [shadcn/ui](https://ui.shadcn.com/) New York style as its component foundation, configured in `packages/ui/components.json`:

```json
{
  "style": "new-york",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "css": "src/styles/globals.css",
    "baseColor": "neutral",
    "cssVariables": true
  }
}
```

On top of shadcn/ui primitives, the package depends on `shadcn-glass-ui` (v2.11+), which provides glassmorphism-aware component variants (`GlassCard`, `InputGlass`, `ButtonGlass`, etc.) and an OKLCH design token system. The integration is declared in `packages/ui/package.json`:

```json
"dependencies": {
  "shadcn-glass-ui": "^2.11.2",
  "tailwindcss": "^4.1.18",
  "@tailwindcss/vite": "^4.1.18"
}
```

The global stylesheet imports shadcn-glass-ui styles first, then Tailwind:

```css
@import "shadcn-glass-ui/styles.css";
@import "tailwindcss";
```

## OKLCH Token System

The design system uses a three-tier token architecture, all defined in CSS custom properties:

### 1. OKLCH Primitives

Low-level color values using the OKLCH color space for perceptual uniformity. These are never used directly by components:

```css
--oklch-white-8: oklch(100% 0 0 / 0.08);
--oklch-copper-500: oklch(62% .16 48);
--oklch-slate-800: oklch(27.9% .041 260);
--oklch-slate-900: oklch(20.8% .042 265);
```

The full set covers white opacity variants (3% through 90%), black variants, and color ramps for copper, bronze, slate, rose, emerald, blue, and amber.

### 2. Semantic Tokens

Mid-level tokens that map primitives to UI roles. These are theme-aware and provide the vocabulary for component tokens. Aurora uses slate-tinted surfaces and borders rather than white-opacity overlays:

```css
--semantic-primary: var(--oklch-copper-500);
--semantic-surface: var(--oklch-slate-800-40);
--semantic-border: var(--oklch-slate-700-40);
--semantic-text: var(--oklch-white-90);
--semantic-glass-bg: var(--oklch-slate-800-40);
```

Semantic tokens cover surfaces (`surface`, `surface-muted`, `surface-elevated`, `surface-overlay`), borders (`border`, `border-muted`, `border-strong`), text (`text`, `text-muted`, `text-subtle`, `text-disabled`), and status colors (success, warning, error, info), each with base, muted, subtle, and text variants.

### 3. Component Tokens

High-level tokens consumed directly by UI components:

```css
/* Card */
--card-bg: var(--semantic-surface-muted);
--card-hover-glow: 0 0 20px var(--semantic-primary-muted);

/* Button */
--btn-primary-bg: linear-gradient(135deg, var(--semantic-primary), var(--semantic-secondary));
--btn-primary-glow: var(--glow-primary);

/* Input */
--input-bg: var(--semantic-surface);
--focus-glow: 0 0 0 2px var(--semantic-primary-muted), 0 0 20px var(--semantic-primary-subtle);
```

### Glass-Specific Tokens

Blur, glow, and animation tokens provide the glassmorphism visual layer:

```css
--blur: 16px;
--blur-glass: calc(var(--blur) * 1.5);
--glow-primary: 0 0 25px var(--oklch-copper-500-40);
--glow-neutral: 0 4px 12px var(--oklch-black-15);

/* Background gradient orbs (aurora: subtle slate-to-slate gradient) */
--bg-from: var(--oklch-slate-950);
--bg-via: var(--oklch-slate-900);
--bg-to: var(--oklch-slate-950);
--orb-1: var(--oklch-blue-500-20);
--orb-2: var(--oklch-copper-600-20);
```

## Theme Definitions

Two active themes are defined. The HSL tokens in `packages/ui/src/styles/globals.css` provide shadcn/ui compatibility; the `data-theme` attribute activates the OKLCH semantic layer from shadcn-glass-ui.

### Light (default for `:root`)

```css
:root {
  --background: 0 0% 100%;
  --primary: 22 75% 50%;
  --card: 0 0% 100%;
  --border: 214 32% 91%;
}
```

Clean white background with copper accents. No alpha transparency on surfaces; solid backgrounds replace glassmorphism blur.

### Aurora (dark)

```css
.dark {
  --background: 222 84% 5%;
  --primary: 22 75% 50%;
  --card: 222 47% 11%;
  --border: 217 33% 17%;
}
```

Deep dark with copper/bronze glow effects and slate-tinted surfaces. Aurora uses opaque HSL base values; transparency is managed at the OKLCH semantic layer through `--oklch-slate-800-*` tokens.

A shared `.glass-surface` utility class applies backdrop-filter blur to any layout element:

```css
.glass-surface {
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}
```

## ThemeProvider

The `ThemeProvider` in `packages/ui/src/context/ThemeContext.tsx` manages theme state:

```tsx
export type Theme = "dark" | "light" | "system";

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const raw = getThemeCookie() ?? localStorage.getItem(STORAGE_KEY);
    return raw ? migrateTheme(raw) : "system";
  });

  useEffect(() => {
    const isDark = theme === "dark" || (theme === "system" && prefersDark);
    document.documentElement.classList.toggle("dark", isDark);
    document.documentElement.setAttribute("data-theme", isDark ? "aurora" : "light");
    document.documentElement.style.colorScheme = isDark ? "dark" : "light";
  }, [theme]);
  // ...
}
```

Key behaviors:
- Persists to `localStorage` and HTTP cookie under the key `neokapi-theme`
- Defaults to `"system"` when no stored preference exists
- Sets `data-theme="aurora"` for dark mode, `data-theme="light"` for light mode
- Toggles `dark` class for Tailwind dark-mode utilities
- Sets `colorScheme` style for native browser controls (form inputs, scrollbars)
- Cookie shared across subdomains for cross-app consistency (domain detection logic)
- Legacy `"glass"` and `"aurora"` string values migrated to `"dark"`

Theme selection UI lives in the Bowrain desktop app's Settings page (`SettingsPage.tsx`), which renders toggle buttons for Light, Dark, and System.

## AnimatedBackgroundGlass

The `AnimatedBackgroundGlass` component (`packages/ui/src/components/ui/animated-background.tsx`) renders the background layer with animated floating orbs:

```tsx
export function AnimatedBackgroundGlass({ className, showCenterOrb }: AnimatedBackgroundProps) {
  const { theme } = useTheme();
  const shouldShowCenterOrb = showCenterOrb ?? theme === "dark";

  return (
    <div style={{ background: "linear-gradient(135deg, var(--bg-from), var(--bg-via), var(--bg-to))" }}>
      {/* 4-5 floating orbs with blur filters and staggered animation delays */}
    </div>
  );
}
```

The orbs use `--orb-1` through `--orb-5` CSS variables with large blur filters (60-100px) and the `orb-float` keyframe animation. Aurora uses blue/copper/cyan orbs with a subtle slate gradient background (no center orb by default since `--orb-5` is transparent).

## Cross-Project Theme Application

All four frontend projects consume the theme system identically:

1. **CSS import**: Each project's `index.css` imports the shared globals:
   ```css
   @import "@neokapi/ui/styles/globals.css";
   ```

2. **Vite alias**: Each project's `vite.config.ts` maps `@neokapi/ui` to the shared package source:
   ```ts
   resolve: {
     alias: {
       "@neokapi/ui": path.resolve(__dirname, "../../../packages/ui/src"),
     },
   },
   ```

3. **ThemeProvider wrapper**: Each project wraps its root component:
   ```tsx
   <ThemeProvider>
     <App />
   </ThemeProvider>
   ```

| Project | CSS Entry | ThemeProvider Location |
|---------|-----------|----------------------|
| Bowrain Desktop | `bowrain/apps/bowrain/frontend/src/index.css` | `App.tsx` |
| Web App | `bowrain/apps/web/src/index.css` | `App.tsx` |
| Kapi Web | `kapi/apps/kapi-web/src/index.css` | `App.tsx` |
| Keycloak Theme | `bowrain/apps/keycloak-theme/src/login/main.css` | `main.tsx` |

The Keycloak theme is a special case: it imports `@neokapi/ui/styles/globals.css` but also re-declares approximately 135 additional CSS custom properties (full OKLCH color palettes, glass surfaces, component variants, effects) in `:root` to work around Keycloakify's CSS processing, which can strip or reorder `@layer` blocks. The shared `packages/ui` defines ~76 properties; the Keycloak theme adds ~59 more for the complete glass design system. See [Keycloak Theming](keycloak-theming.md) for details.

---
sidebar_position: 9
title: "Glass UI Theme System"
---
# Glass UI Theme System

This note documents the glassmorphism-based theme system used across all four frontend projects. The design system builds on `shadcn-glass-ui` and Tailwind CSS v4, with an OKLCH-based token hierarchy and three named themes.

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

## OKLCH Glassmorphism Token System

The design system uses a three-tier token architecture, all defined in CSS custom properties:

### 1. OKLCH Primitives

Low-level color values using the OKLCH color space for perceptual uniformity. These are never used directly by components:

```css
--oklch-white-8: oklch(100% 0 0 / 0.08);
--oklch-purple-500: oklch(66.6% .159 303);
--oklch-slate-900: oklch(20.8% .042 265);
```

The full set covers white opacity variants (3% through 90%), black variants, and color ramps for purple, violet, slate, rose, emerald, blue, and amber.

### 2. Semantic Tokens

Mid-level tokens that map primitives to UI roles. These are theme-aware and provide the vocabulary for component tokens:

```css
--semantic-primary: var(--oklch-purple-500);
--semantic-surface: var(--oklch-white-8);
--semantic-border: var(--oklch-white-15);
--semantic-text: var(--oklch-white-90);
--semantic-glass-bg: var(--oklch-white-5);
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
--glow-primary: 0 0 30px var(--oklch-purple-500-60);
--glow-neutral: 0 4px 15px var(--oklch-black-20);

/* Background gradient orbs */
--bg-from: var(--oklch-slate-900);
--bg-via: var(--oklch-purple-700);
--bg-to: var(--oklch-slate-900);
--orb-1: var(--oklch-purple-500-30);
--orb-2: var(--oklch-blue-500-20);
```

## Theme Definitions

Three themes are defined in `packages/ui/src/styles/globals.css` using HSL CSS variables for shadcn/ui compatibility and `data-theme` attribute selectors:

### Glass (default, dark)

```css
:root,
[data-theme="glass"] {
  --background: 222 47% 11%;
  --primary: 271 91% 65%;
  --card: 222 84% 5% / 0.6;
  --border: 0 0% 100% / 0.12;
}
```

Dark navy background with purple accents. Cards and surfaces use alpha transparency for backdrop-filter glassmorphism effects.

### Light

```css
[data-theme="light"] {
  --background: 0 0% 100%;
  --primary: 263 70% 50%;
  --card: 0 0% 100%;
  --border: 214 32% 91%;
}
```

Clean white background with violet accents. No alpha transparency on surfaces; solid backgrounds replace glassmorphism blur.

### Aurora (dark)

```css
[data-theme="aurora"] {
  --background: 222 84% 5%;
  --primary: 263 70% 50%;
  --card: 222 47% 11% / 0.5;
  --border: 0 0% 100% / 0.1;
}
```

Deeper dark with violet/purple glow effects. Lower alpha values on surfaces for a more transparent, ethereal appearance compared to Glass.

A shared `.glass-surface` utility class applies backdrop-filter blur to any layout element:

```css
.glass-surface {
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}
```

## ThemeProvider and Theme Cycling

The `ThemeProvider` in `packages/ui/src/context/ThemeContext.tsx` manages theme state:

```tsx
export type Theme = "glass" | "light" | "aurora";

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const stored = localStorage.getItem("gokapi-theme");
    return stored && isValidTheme(stored) ? stored : "glass";
  });

  useEffect(() => {
    const isDark = theme !== "light";
    document.documentElement.classList.toggle("dark", isDark);
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  // ...
}
```

Key behaviors:
- Persists to `localStorage` under the key `gokapi-theme`
- Defaults to `"glass"` when no stored preference exists
- Sets `data-theme` attribute on `<html>` for CSS variable resolution
- Toggles `dark` class for Tailwind dark-mode utilities (Glass and Aurora are dark-based)

Theme selection UI lives in the Bowrain desktop app's Settings page (`SettingsPage.tsx`), which renders toggle buttons for Glass, Light, and Aurora.

## AnimatedBackgroundGlass

The `AnimatedBackgroundGlass` component (`packages/ui/src/components/ui/animated-background.tsx`) renders the glassmorphism background layer with animated floating orbs:

```tsx
export function AnimatedBackgroundGlass({ className, showCenterOrb }: AnimatedBackgroundProps) {
  const { theme } = useTheme();
  const shouldShowCenterOrb = showCenterOrb ?? theme === "glass";

  return (
    <div style={{ background: "linear-gradient(135deg, var(--bg-from), var(--bg-via), var(--bg-to))" }}>
      {/* 4-5 floating orbs with blur filters and staggered animation delays */}
    </div>
  );
}
```

The orbs use `--orb-1` through `--orb-5` CSS variables with large blur filters (60-100px) and the `orb-float` keyframe animation. The center orb is conditionally shown based on the active theme.

## Cross-Project Theme Application

All four frontend projects consume the theme system identically:

1. **CSS import**: Each project's `index.css` imports the shared globals:
   ```css
   @import "@gokapi/ui/styles/globals.css";
   ```

2. **Vite alias**: Each project's `vite.config.ts` maps `@gokapi/ui` to the shared package source:
   ```ts
   resolve: {
     alias: {
       "@gokapi/ui": path.resolve(__dirname, "../../../packages/ui/src"),
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
| Kapi Web | `bowrain/apps/kapi-web/src/index.css` | `App.tsx` |
| Keycloak Theme | `bowrain/apps/keycloak-theme/src/login/main.css` | `main.tsx` |

The Keycloak theme is a special case: it imports `@gokapi/ui/styles/globals.css` but also re-declares all OKLCH tokens and semantic tokens in `:root` to work around Keycloakify's CSS processing, which can strip or reorder `@layer` blocks. See [Keycloak Theming](keycloak-theming.md) for details.

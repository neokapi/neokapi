/**
 * Flow editor theme — maps semantic tokens to CSS custom property references.
 * The host app (kapi-desktop, bowrain, etc.) provides the actual values
 * via its globals.css / index.css theme definition.
 *
 * Uses CSS custom property references so the flow editor automatically
 * inherits the host app's theme (Kapi Blue, Sandstone, etc.).
 */

export const theme = {
  // Surfaces
  bg: "var(--background)",
  bgCard: "var(--card)",
  bgMuted: "var(--muted)",
  bgSecondary: "var(--secondary)",

  // Foreground / text
  fg: "var(--foreground)",
  fgMuted: "var(--muted-foreground)",
  fgSecondary: "var(--secondary-foreground)",

  // Borders
  border: "var(--border)",
  input: "var(--input)",

  // Accent / ring
  accent: "var(--accent)",
  accentFg: "var(--accent-foreground)",
  ring: "var(--ring)",

  // Primary
  primary: "var(--primary)",
  primaryFg: "var(--primary-foreground)",

  // Destructive
  destructive: "var(--destructive)",
  destructiveFg: "var(--destructive-foreground)",
} as const;

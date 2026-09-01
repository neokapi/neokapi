import { localeLabel, resolveLocaleName } from "../../lib/locale-name";

interface LocalePillProps {
  locale: string;
  className?: string;
  /**
   * Render the pill in a neutral grey instead of its color-coded hue — used to
   * de-emphasise locales outside the active language filter, so only the
   * filtered-in locales keep their colour.
   */
  muted?: boolean;
  /**
   * Render the language name beside the pill. Use it wherever there is room:
   * a dropdown item, a list row, a heading. The bare pill is for dense grids
   * (a coverage matrix, a table cell) where a name does not fit; there the
   * title carries it.
   */
  showName?: boolean;
}

/** Hash a string to a consistent hue angle (0-360). */
function localeHue(locale: string): number {
  let hash = 0;
  for (let i = 0; i < locale.length; i++) {
    hash = locale.charCodeAt(i) + ((hash << 5) - hash);
  }
  return Math.abs(hash) % 360;
}

/**
 * Compact locale badge with subtle color-coded background.
 * Color is deterministic based on the locale code.
 * Adjusts lightness for dark mode via CSS custom properties.
 *
 * The pill always carries the language name in its title, so a reader who does
 * not know the tag can reach it; `showName` puts the name on the page beside it.
 */
export function LocalePill({ locale, className, muted, showName }: LocalePillProps) {
  const hue = localeHue(locale);
  const name = resolveLocaleName(locale);

  const pill = (
    <span
      title={showName ? undefined : localeLabel(locale)}
      className={`inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium ${showName ? "" : (className ?? "")}`}
      style={
        muted
          ? {
              // Neutral grey chip with softened text — the design system's muted
              // foreground, so the de-emphasised pill reads as clearly inactive
              // and adapts to light/dark. Background stays a chroma-0 grey at the
              // shared pill lightness.
              backgroundColor: `oklch(var(--pill-bg-l, 0.92) 0 0)`,
              color: "var(--muted-foreground)",
            }
          : {
              backgroundColor: `oklch(var(--pill-bg-l, 0.92) 0.03 ${hue})`,
              color: `oklch(var(--pill-fg-l, 0.4) 0.08 ${hue})`,
            }
      }
    >
      {locale}
    </span>
  );

  if (!showName) return pill;

  return (
    <span className={`inline-flex min-w-0 items-center gap-1.5 ${className ?? ""}`}>
      {pill}
      <span className="truncate">{name}</span>
    </span>
  );
}

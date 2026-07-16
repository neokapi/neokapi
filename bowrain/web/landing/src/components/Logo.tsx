// The bowrain mark — a perfect quarter-circle rainbow (square-cut ends).
// Drawn inline so it recolors with the active theme's prism ramp; the
// canonical standalone vector lives at bowrain/assets/brand/mark.svg and
// fans out to all raster assets via scripts/generate-bowrain-brand-assets.sh.
export function Logo({ className = "h-7 w-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" fill="none" className={className} aria-hidden="true">
      <defs>
        <linearGradient id="bowmark" x1="6" y1="10" x2="54" y2="54" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="var(--prism-1)" />
          <stop offset="0.3" stopColor="var(--prism-2)" />
          <stop offset="0.55" stopColor="var(--prism-3)" />
          <stop offset="0.8" stopColor="var(--prism-4)" />
          <stop offset="1" stopColor="var(--prism-5)" />
        </linearGradient>
      </defs>
      <path d="M6 6 A52 52 0 0 1 58 58 L38 58 A32 32 0 0 0 6 26 Z" fill="url(#bowmark)" />
    </svg>
  );
}

// The bow motif as a wide atmospheric arc, used behind the hero.
export function BowArc({ className = "" }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 1200 500"
      fill="none"
      className={className}
      aria-hidden="true"
      preserveAspectRatio="xMidYMax slice"
    >
      <defs>
        <linearGradient
          id="bow-arc"
          x1="0"
          y1="500"
          x2="1200"
          y2="500"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0" stopColor="var(--prism-1)" stopOpacity="0" />
          <stop offset="0.2" stopColor="var(--prism-1)" />
          <stop offset="0.4" stopColor="var(--prism-2)" />
          <stop offset="0.6" stopColor="var(--prism-3)" />
          <stop offset="0.8" stopColor="var(--prism-4)" />
          <stop offset="1" stopColor="var(--prism-5)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d="M40 520 A560 560 0 0 1 1160 520" stroke="url(#bow-arc)" strokeWidth="2.5" />
      <path
        d="M100 520 A500 500 0 0 1 1100 520"
        stroke="url(#bow-arc)"
        strokeWidth="1.5"
        opacity="0.5"
      />
      <path
        d="M160 520 A440 440 0 0 1 1040 520"
        stroke="url(#bow-arc)"
        strokeWidth="1"
        opacity="0.25"
      />
    </svg>
  );
}

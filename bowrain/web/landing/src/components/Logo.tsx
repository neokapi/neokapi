// The bowrain mark — a bow of prismatic bands sheltering a single raindrop.
// Drawn inline so it recolors with the active theme's prism ramp.
export function Logo({ className = "h-7 w-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" fill="none" className={className} aria-hidden="true">
      <path
        d="M6 44 A26 26 0 0 1 58 44"
        stroke="var(--prism-5)"
        strokeWidth="6"
        strokeLinecap="round"
      />
      <path
        d="M13 44 A19 19 0 0 1 51 44"
        stroke="var(--prism-4)"
        strokeWidth="6"
        strokeLinecap="round"
      />
      <path
        d="M20 44 A12 12 0 0 1 44 44"
        stroke="var(--prism-3)"
        strokeWidth="6"
        strokeLinecap="round"
      />
      <path
        d="M32 32 C32 32 25 41 25 46 a7 7 0 0 0 14 0 C39 41 32 32 32 32 Z"
        fill="var(--prism-4)"
      />
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

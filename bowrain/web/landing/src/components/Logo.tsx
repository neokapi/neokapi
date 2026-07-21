// The bowrain mark, for inline UI use (navbar, footer, surface cards). A PNG
// rather than the inline SVG: mark.svg is the full illustration (~300KB of
// path data with a per-shape soft-focus filter), too heavy to embed on every
// page. mark.png is a 256px transparent render of it, regenerated alongside
// every other derived asset by scripts/generate-bowrain-brand-assets.sh from
// the canonical bowrain/assets/brand/mark.svg — update that file and re-run
// rather than editing mark.png directly.
export function Logo({ className = "h-7 w-7" }: { className?: string }) {
  return <img src="/mark.png" alt="" className={className} aria-hidden="true" />;
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

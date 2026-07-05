import React from "react";

// LabPageShell — the shared chrome for every lab page: a centered column with
// a title + one-paragraph lede, wrapped in the `.kapi-reference` token scope so
// the ui-primitives/tailwind styling the labs (and the shared RunGate) use
// resolves in both themes. Replaces the per-page *.module.css chrome that each
// lab previously duplicated.
//
// `.kapi-reference` zeroes heading margins/sizes (see src/css/tailwind.css), so
// the h1/lede are styled explicitly with utilities here.

export interface LabPageShellProps {
  /** Page heading (sentence case). */
  title: React.ReactNode;
  /** One-paragraph lede under the heading. */
  lede: React.ReactNode;
  /** Optional extra hero content (e.g. a secondary note or tab row). */
  heroExtra?: React.ReactNode;
  /** Max content width utility (default `max-w-[1080px]`). */
  maxWidthClassName?: string;
  children: React.ReactNode;
}

export function LabPageShell({
  title,
  lede,
  heroExtra,
  maxWidthClassName = "max-w-[1080px]",
  children,
}: LabPageShellProps): React.ReactElement {
  return (
    <main className={`kapi-reference mx-auto w-full min-w-0 ${maxWidthClassName} px-4 pb-16 pt-10`}>
      <header className="mb-8">
        <h1 className="mb-2 text-3xl font-bold text-foreground">{title}</h1>
        <p className="max-w-[78ch] text-[1.05rem] leading-relaxed text-muted-foreground">{lede}</p>
        {heroExtra}
      </header>
      {children}
    </main>
  );
}

/** Muted footnote block used under a lab (cross-links, caveats). */
export function LabFootnote({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <p className="mt-6 border-t pt-5 text-sm leading-relaxed text-muted-foreground">{children}</p>
  );
}

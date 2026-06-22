import React, { useEffect } from "react";
import { registerBlock } from "./store";

// <Cli> / <Desktop> wrap surface-specific content in a doc. They render a tagged
// div (shown/hidden by global CSS keyed on html[data-surface]) and register
// their presence so the navbar SurfaceToggle appears only on dual-mode pages.
// Registered globally in src/theme/MDXComponents.tsx, so authors use <Cli>…</Cli>
// and <Desktop>…</Desktop> with no import.
function SurfaceBlock({
  kind,
  label,
  children,
}: {
  kind: "cli" | "desktop";
  label: string;
  children: React.ReactNode;
}): React.ReactElement {
  useEffect(() => registerBlock(), []);
  return (
    <div className="surface-block" data-surface-kind={kind} data-surface-label={label}>
      {children}
    </div>
  );
}

export function Cli({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <SurfaceBlock kind="cli" label="CLI">
      {children}
    </SurfaceBlock>
  );
}

export function Desktop({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <SurfaceBlock kind="desktop" label="Kapi Desktop">
      {children}
    </SurfaceBlock>
  );
}

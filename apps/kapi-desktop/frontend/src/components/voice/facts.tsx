// The fact grid the Voice page reads a profile with: a row of small
// uppercase-labelled values, at one rhythm, so tone, style and the per-language
// overrides all line up rather than each inventing its own grid.

import { cn } from "@neokapi/ui-primitives";

/** One value in a fact grid: absent when it has nothing to show. */
export interface FactItem {
  label: string;
  value?: string;
}

/** A labelled value in a compact definition grid. */
export function Fact({ label, value }: FactItem) {
  if (!value) return null;
  return (
    <div className="min-w-0" data-testid="voice-fact">
      <dt className="text-[11px] tracking-wide text-muted-foreground uppercase">{label}</dt>
      <dd className="truncate text-sm text-foreground">{value}</dd>
    </div>
  );
}

/**
 * A grid of facts, or nothing when none of them has a value.
 *
 * `columns` is the widest the grid grows to on a roomy viewport; it always
 * starts at two so a fact never sits alone on a narrow one.
 */
export function FactGrid({ facts, columns = 3 }: { facts: FactItem[]; columns?: 3 | 4 }) {
  if (!facts.some((f) => f.value)) return null;
  return (
    <dl
      className={cn(
        "grid grid-cols-2 gap-x-4 gap-y-2",
        columns === 4 ? "sm:grid-cols-4" : "sm:grid-cols-3",
      )}
      data-testid="voice-fact-grid"
    >
      {facts.map((f) => (
        <Fact key={f.label} label={f.label} value={f.value} />
      ))}
    </dl>
  );
}

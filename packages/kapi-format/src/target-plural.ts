/**
 * @neokapi/kapi-format — translator-side plural helpers.
 *
 * A translator looking at a flat target (`Sie haben {count} Nachrichten`)
 * may realize the locale needs plural dispatch, even though the source
 * never used `<Plural>`. This module provides the pure data-model
 * operations an editor UI can call to flip a Block's target for a
 * single locale between flat and structured `PluralRun` shapes, and
 * back again. Source stays untouched.
 *
 * No React, no DOM, no IO — the helpers operate on the Run shapes
 * alone so they're reusable across kapi-desktop, bowrain's web
 * editor, and third-party CAT tools that embed the primitives.
 */

import type { Block, PluralForm, PluralRunWrapper, Run } from './block.ts';

/** Candidates from a Block's placeholders for pivot selection. */
export interface PluralPivotCandidate {
  /** Placeholder `name`, which is the equiv a form body should reference. */
  name: string;
  /** Free-form label to show in the picker, usually `name + (jsType)`. */
  label: string;
  /** Whether the placeholder is already flagged as an ICU pivot in the source. */
  sourcePivot: boolean;
}

/**
 * Enumerates placeholder candidates a translator can pick as a
 * plural pivot. Prefers numeric-typed placeholders, then any
 * already-marked `icu-pivot`, then any remaining placeholder. Returns
 * an ordered list; callers can pre-select the first entry.
 */
export function pluralPivotCandidates(block: Block): PluralPivotCandidate[] {
  const seen = new Set<string>();
  const out: PluralPivotCandidate[] = [];
  const byPriority = [...block.placeholders].sort((a, b) => rank(a) - rank(b));
  for (const p of byPriority) {
    if (seen.has(p.name)) continue;
    seen.add(p.name);
    const suffix = p.jsType ? ` (${p.jsType})` : '';
    out.push({
      name: p.name,
      label: `${p.name}${suffix}`,
      sourcePivot: p.kind === 'icu-pivot',
    });
  }
  return out;
}

// Smaller rank → higher priority in the candidate picker.
function rank(p: Block['placeholders'][number]): number {
  if (p.kind === 'icu-pivot') return 0;
  if (p.jsType === 'number') return 1;
  return 2;
}

/**
 * Upgrade a flat target Run sequence into a structured `PluralRun`.
 * The existing flat runs become the `other` form; every other plural
 * form is initialized empty for the translator to fill. Idempotent:
 * passing an already-plural target returns it unchanged.
 */
export function upgradeTargetToPlural(
  target: readonly Run[] | undefined,
  pivot: string,
  forms: readonly PluralForm[] = ['zero', 'one', 'two', 'few', 'many', 'other'],
): Run[] {
  if (isPlural(target ?? [])) return [...(target ?? [])];
  const existing = target ? [...target] : [];
  const formsMap: Partial<Record<PluralForm, Run[]>> = {};
  for (const form of forms) {
    formsMap[form] = form === 'other' ? existing : [];
  }
  if (!formsMap.other) formsMap.other = existing;
  const wrapper: PluralRunWrapper = {
    plural: { pivot, forms: formsMap },
  };
  return [wrapper];
}

/**
 * Collapse a `PluralRun` target back to its `other` branch's flat
 * runs. Returns the runs unchanged when the target isn't a plural.
 * Loses every form except `other` — the caller is expected to
 * confirm with the translator first.
 */
export function downgradePluralTarget(target: readonly Run[] | undefined): Run[] {
  if (!target || target.length === 0) return [];
  const first = target[0];
  if (!isPluralWrapper(first)) return [...target];
  return [...(first.plural.forms.other ?? [])];
}

/**
 * Returns true when the target begins with a `PluralRun` — the
 * only shape the translator-side upgrade produces.
 */
export function isPlural(target: readonly Run[]): boolean {
  return target.length > 0 && isPluralWrapper(target[0]);
}

function isPluralWrapper(run: Run): run is PluralRunWrapper {
  return 'plural' in run;
}

/**
 * Read the current pivot from a plural target, or null when the
 * target isn't structured.
 */
export function pluralTargetPivot(target: readonly Run[]): string | null {
  if (target.length === 0) return null;
  const first = target[0];
  if (!isPluralWrapper(first)) return null;
  return first.plural.pivot;
}

/**
 * Replace a single form in a plural target, returning a new target
 * array. Other forms, the pivot, and non-plural runs are untouched.
 * Creates the form if absent. No-ops when the target isn't plural.
 */
export function setPluralForm(
  target: readonly Run[],
  form: PluralForm,
  formRuns: Run[],
): Run[] {
  if (target.length === 0) return [...target];
  const first = target[0];
  if (!isPluralWrapper(first)) return [...target];
  const nextForms: Partial<Record<PluralForm, Run[]>> = {
    ...first.plural.forms,
    [form]: formRuns,
  };
  return [
    {
      plural: { pivot: first.plural.pivot, forms: nextForms },
    } satisfies PluralRunWrapper,
    ...target.slice(1),
  ];
}

/**
 * `<PluralTargetEditor>` — the translator-facing affordance for
 * authoring plural targets on top of Framework AD-002 Blocks.
 *
 * Two renders, one state:
 *   - Flat: single textarea, Upgrade to plural… button.
 *   - Plural: one textarea per CLDR form (zero, one, …), plus a
 *     Flatten back button that collapses to the `other` form.
 *
 * Every edit emits a new `Run[]` through `onChange`. Parent wires
 * that to whatever backend has the authoritative target (gRPC
 * UpdateBlockTarget, the kapi-desktop backend, a local buffer —
 * this component is storage-agnostic).
 *
 * Data model helpers live in `@neokapi/kapi-format/target-plural`;
 * the Runs↔text conversion lives in `./runs-text`. This file is
 * presentation only.
 */

import type { ReactElement } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { useMemo, useState } from "react";

import type { Block, PluralForm, Run } from "@neokapi/kapi-format";
import {
  downgradePluralTarget,
  isPlural,
  pluralPivotCandidates,
  pluralTargetPivot,
  setPluralForm,
  upgradeTargetToPlural,
} from "@neokapi/kapi-format";

import { cn } from "../../lib/utils";
import { runsToText, textToRuns } from "./runs-text";

/** Default CLDR plural-form order presented to translators. */
const DEFAULT_FORMS: readonly PluralForm[] = ["zero", "one", "two", "few", "many", "other"];

export interface PluralTargetEditorProps {
  /**
   * The block whose target is being edited. Used for pivot-candidate
   * resolution + placeholder metadata when parsing translator edits
   * back into typed runs. Source target isn't mutated.
   */
  block: Pick<Block, "source" | "placeholders">;
  /** Current translation for the active locale. */
  target: readonly Run[];
  /** Called with the new Run[] whenever the translator edits any form. */
  onChange(next: Run[]): void;
  /** Optional subset of CLDR forms to display; defaults to all six. */
  forms?: readonly PluralForm[];
  /** Pass-through class for the outer container. */
  className?: string;
}

export function PluralTargetEditor(props: PluralTargetEditorProps): ReactElement {
  const forms = props.forms ?? DEFAULT_FORMS;
  return isPlural(props.target) ? (
    <PluralForms {...props} forms={forms} />
  ) : (
    <FlatTarget {...props} forms={forms} />
  );
}

// ─── Flat view ────────────────────────────────────────────────────

function FlatTarget({
  block,
  target,
  onChange,
  forms,
  className,
}: PluralTargetEditorProps & { forms: readonly PluralForm[] }) {
  const candidates = useMemo(() => pluralPivotCandidates(block as Block), [block]);
  const text = useMemo(() => runsToText(target), [target]);
  const preselectedPivot = candidates[0]?.name ?? "";

  const [chosenPivot, setChosenPivot] = useState(preselectedPivot);

  const handleEdit = (next: string) => {
    onChange(textToRuns(next, block.placeholders, block.source));
  };

  const upgrade = () => {
    if (!chosenPivot) return;
    onChange(upgradeTargetToPlural(target, chosenPivot, forms));
  };

  return (
    <div className={cn("space-y-2", className)} data-neokapi-plural-editor="flat">
      <textarea
        className={textareaClass}
        value={text}
        rows={3}
        onChange={(e) => handleEdit(e.target.value)}
        aria-label="Translation"
      />
      {candidates.length > 0 ? (
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Pivot:</span>
          <select
            className={selectClass}
            value={chosenPivot}
            onChange={(e) => setChosenPivot(e.target.value)}
            aria-label="Plural pivot variable"
          >
            {candidates.map((c) => (
              <option key={c.name} value={c.name}>
                {c.label}
                {c.sourcePivot ? t(" — source pivot") : ""}
              </option>
            ))}
          </select>
          <button type="button" className={buttonClass} onClick={upgrade} disabled={!chosenPivot}>
            Upgrade to plural…
          </button>
        </div>
      ) : null}
    </div>
  );
}

// ─── Per-form view ───────────────────────────────────────────────

function PluralForms({
  block,
  target,
  onChange,
  forms,
  className,
}: PluralTargetEditorProps & { forms: readonly PluralForm[] }) {
  const pivot = pluralTargetPivot(target) ?? "";
  const formTexts = useMemo(() => {
    const out = new Map<PluralForm, string>();
    const runs = (target[0] as { plural: { forms: Partial<Record<PluralForm, Run[]>> } }).plural
      .forms;
    for (const form of forms) out.set(form, runsToText(runs[form] ?? []));
    return out;
  }, [target, forms]);

  const editForm = (form: PluralForm, text: string) => {
    const runs = textToRuns(text, block.placeholders, block.source);
    onChange(setPluralForm(target, form, runs));
  };

  const downgrade = () => onChange(downgradePluralTarget(target));

  return (
    <div className={cn("space-y-3", className)} data-neokapi-plural-editor="plural">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-muted-foreground">
          Plural pivot: <span className="font-mono">{pivot}</span>
        </span>
        <button
          type="button"
          className={ghostButtonClass}
          onClick={downgrade}
          aria-label="Collapse plural target to flat text"
        >
          Flatten back to single target
        </button>
      </div>
      <div className="space-y-2">
        {forms.map((form) => (
          <FormRow
            key={form}
            form={form}
            value={formTexts.get(form) ?? ""}
            onEdit={(v) => editForm(form, v)}
          />
        ))}
      </div>
    </div>
  );
}

function FormRow({
  form,
  value,
  onEdit,
}: {
  form: PluralForm;
  value: string;
  onEdit(v: string): void;
}) {
  return (
    <label className="flex gap-3">
      <span className="mt-2 w-20 shrink-0 text-right text-xs uppercase tracking-wide text-muted-foreground">
        {form}
      </span>
      <textarea
        className={textareaClass}
        value={value}
        rows={2}
        onChange={(e) => onEdit(e.target.value)}
        aria-label={`${form} form`}
      />
    </label>
  );
}

// ─── Small styling helpers ───────────────────────────────────────

const textareaClass = cn(
  "min-h-[2.5rem] w-full resize-y rounded-md border border-input bg-background",
  "px-3 py-2 font-mono text-sm placeholder:text-muted-foreground",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
);

const selectClass = cn(
  "h-8 rounded-md border border-input bg-background px-2 text-sm",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
);

const buttonClass = cn(
  "h-8 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground",
  "transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none",
);

const ghostButtonClass = cn(
  "h-8 rounded-md px-3 text-sm text-muted-foreground",
  "hover:bg-accent hover:text-foreground",
);

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

import type { ReactElement, ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { useMemo, useRef, useState } from "react";

import type { Block, PluralForm, Run } from "@neokapi/kapi-format";
import {
  downgradePluralTarget,
  isPlural,
  pluralPivotCandidates,
  pluralTargetPivot,
  setPluralForm,
  sourceRuns,
  upgradeTargetToPlural,
} from "@neokapi/kapi-format";

import { cn } from "../../lib/utils";
import { runsToText, textToRuns } from "./runs-text";

/** Default CLDR plural-form order presented to translators. */
const DEFAULT_FORMS: readonly PluralForm[] = ["zero", "one", "two", "few", "many", "other"];

export interface PluralTargetEditorProps {
  /**
   * The block whose target is being edited. Its source edition and
   * placeholder table resolve pivot candidates and turn a translator's edit
   * back into typed runs. The source is not mutated.
   */
  block: Pick<Block, "editions" | "placeholders">;
  /** Current translation for the active locale. */
  target: readonly Run[];
  /** Called with the new Run[] whenever the translator edits any form. */
  onChange(next: Run[]): void;
  /** Optional subset of CLDR forms to display; defaults to all six. */
  forms?: readonly PluralForm[];
  /**
   * Keep the structure as it is: the forms are edited, and neither "Upgrade to
   * plural" nor "Flatten back" is offered. A host that saves each form as its
   * own change (a branch of the plural, by path) passes it, since a change of
   * structure replaces the whole edition.
   */
  fixedStructure?: boolean;
  /**
   * Draw the editor of one plural form in place of its textarea. The textarea
   * shows each inline code as its `{equiv}` and finds the code again by that
   * text, so a code whose equiv holds a brace, or two codes that share one,
   * do not survive an edit there. A host whose forms hold inline codes passes
   * an editor that keeps each code by its id, such as the inline-code editor,
   * and calls `onEdit` with the form's new runs.
   */
  renderForm?: (form: PluralForm, runs: readonly Run[], onEdit: (runs: Run[]) => void) => ReactNode;
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
  fixedStructure,
  className,
}: PluralTargetEditorProps & { forms: readonly PluralForm[] }) {
  const candidates = useMemo(() => pluralPivotCandidates(block), [block]);
  const text = useMemo(() => runsToText(target), [target]);
  const preselectedPivot = candidates[0]?.name ?? "";

  const [chosenPivot, setChosenPivot] = useState(preselectedPivot);

  const handleEdit = (next: string) => {
    onChange(textToRuns(next, block.placeholders, sourceRuns(block)));
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
      {candidates.length > 0 && !fixedStructure ? (
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
                {c.sourcePivot ? t(" (source pivot)") : ""}
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
  fixedStructure,
  renderForm,
  className,
}: PluralTargetEditorProps & { forms: readonly PluralForm[] }) {
  const pivot = pluralTargetPivot(target) ?? "";
  const formRuns = (target[0] as { plural: { forms: Partial<Record<PluralForm, Run[]>> } }).plural
    .forms;
  const formTexts = useMemo(() => {
    const out = new Map<PluralForm, string>();
    for (const form of forms) out.set(form, runsToText(formRuns[form] ?? []));
    return out;
  }, [formRuns, forms]);

  // A form's editor may hold the callback it was first given, so an edit is
  // applied to the target as it stands, with every other form's edits in it.
  const latest = useRef(target);
  latest.current = target;
  const editRuns = (form: PluralForm, runs: Run[]) =>
    onChange(setPluralForm(latest.current, form, runs));
  const editForm = (form: PluralForm, text: string) =>
    editRuns(form, textToRuns(text, block.placeholders, sourceRuns(block)));

  const downgrade = () => onChange(downgradePluralTarget(target));

  return (
    <div className={cn("space-y-3", className)} data-neokapi-plural-editor="plural">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-muted-foreground">
          Plural pivot: <span className="font-mono">{pivot}</span>
        </span>
        {!fixedStructure && (
          <button
            type="button"
            className={ghostButtonClass}
            onClick={downgrade}
            aria-label="Collapse plural target to flat text"
          >
            Flatten back to single target
          </button>
        )}
      </div>
      <div className="space-y-2">
        {forms.map((form) =>
          renderForm ? (
            <FormSlot key={form} form={form}>
              {renderForm(form, formRuns[form] ?? [], (runs) => editRuns(form, runs))}
            </FormSlot>
          ) : (
            <FormRow
              key={form}
              form={form}
              value={formTexts.get(form) ?? ""}
              onEdit={(v) => editForm(form, v)}
            />
          ),
        )}
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

function FormSlot({ form, children }: { form: PluralForm; children: ReactNode }) {
  return (
    <div className="flex gap-3" data-neokapi-plural-form={form}>
      <span className="mt-2 w-20 shrink-0 text-right text-xs uppercase tracking-wide text-muted-foreground">
        {form}
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
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

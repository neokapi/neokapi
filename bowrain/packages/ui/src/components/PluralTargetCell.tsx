/**
 * `<PluralTargetCell>` — string ↔ Run[] bridge for bowrain's
 * TranslationEditor.
 *
 * Bowrain stores targets as plain strings in `BlockInfo.targets[locale]`.
 * The kapi-react runtime auto-detects `{pivot, plural, …}` in those
 * strings and dispatches through `resolveICU`, so plural targets can
 * be stored today without a proto/wire change — this component is
 * the translator-facing UI for authoring them.
 *
 * Round-trip:
 *   - On open: `parseICUPluralString` reconstructs a `PluralRun` from
 *     the stored ICU string, or falls back to a flat text `Run[]`.
 *   - On save: `flattenRuns` serialises the edited `Run[]` back to a
 *     plain string (plain text for flat targets, ICU syntax for plurals).
 *
 * Adapter: synthesises a minimal `Pick<Block, "source" | "placeholders">`
 * from `BlockInfo` so `PluralTargetEditor` can surface pivot candidates
 * from the block's spans. See issue #408.
 */

import { useMemo, useState } from "react";

import type { Block, Placeholder, Run } from "@neokapi/kapi-format";
import { flattenRuns, parseICUPluralString } from "@neokapi/kapi-format";

import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  PluralTargetEditor,
} from "@neokapi/ui-primitives";

import type { BlockInfo, SpanInfo } from "../types/api";

export interface PluralTargetCellProps {
  /** Bowrain block. `block.targets[locale]` seeds the editor. */
  block: BlockInfo;
  /** The locale whose target is being edited. */
  locale: string;
  /** Called with the serialised string when the translator saves. */
  onSave: (next: string) => void | Promise<void>;
  /** Called when the translator dismisses without saving. */
  onCancel: () => void;
  /** Controls the enclosing dialog's open state. */
  open: boolean;
}

export function PluralTargetCell({ block, locale, onSave, onCancel, open }: PluralTargetCellProps) {
  // `adaptedBlock` depends only on fields TranslationEditor already has,
  // so re-renders don't reshuffle the pivot candidates underfoot.
  const adaptedBlock = useMemo(() => toKapiBlock(block), [block]);

  const [runs, setRuns] = useState<Run[]>(() => initialRunsFor(block.targets[locale] ?? ""));
  const [saving, setSaving] = useState(false);

  // Reset the buffer whenever the dialog is reopened — e.g. the
  // translator edits a different block.
  const [seenBlockKey, setSeenBlockKey] = useState<string>(
    `${block.id}|${locale}|${block.targets[locale] ?? ""}`,
  );
  const blockKey = `${block.id}|${locale}|${block.targets[locale] ?? ""}`;
  if (blockKey !== seenBlockKey) {
    setSeenBlockKey(blockKey);
    setRuns(initialRunsFor(block.targets[locale] ?? ""));
  }

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(flattenRuns(runs));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onCancel())}>
      <DialogContent className="max-w-2xl" data-testid="plural-target-dialog">
        <DialogHeader>
          <DialogTitle>Edit plural target</DialogTitle>
          <DialogDescription>
            Upgrade this {locale} target into per-form variants, or keep it flat. The saved value
            always round-trips through ICU plural syntax so the runtime picks the right form at
            render time.
          </DialogDescription>
        </DialogHeader>

        <div className="py-2">
          <PluralTargetEditor block={adaptedBlock} target={runs} onChange={setRuns} />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onCancel} disabled={saving}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving} data-testid="plural-save">
            {saving ? "Saving…" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ─── Internals ───────────────────────────────────────────────────

/**
 * Seed the editor's Run[] buffer from the raw string stored on the
 * block. ICU plural strings become structured plural runs so the
 * per-form view opens directly; everything else becomes a single
 * TextRun — the translator can still click "Upgrade to plural…" to
 * convert it.
 */
function initialRunsFor(raw: string): Run[] {
  const parsed = parseICUPluralString(raw);
  if (parsed) return parsed;
  return raw ? [{ text: raw }] : [];
}

/**
 * Build the `Pick<Block, "source" | "placeholders">` the editor
 * needs. Bowrain's BlockInfo doesn't carry typed Placeholder data;
 * we synthesise the placeholder table from `source_spans` so
 * pivot-candidate dropdowns still work — numeric-equiv spans
 * surface as the natural candidates first (see
 * `pluralPivotCandidates` in @neokapi/kapi-format/target-plural).
 */
export function toKapiBlock(block: BlockInfo): Pick<Block, "source" | "placeholders"> {
  return {
    source: [{ text: block.source }],
    placeholders: spansToPlaceholders(block.source_spans),
  };
}

function spansToPlaceholders(spans: readonly SpanInfo[] | undefined): Placeholder[] {
  if (!spans) return [];
  const out: Placeholder[] = [];
  const seen = new Set<string>();
  for (const span of spans) {
    // Paired codes contribute both opening + closing; each half has
    // the same `equiv`, so dedupe by name.
    const name = span.equiv_text?.trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    out.push({
      name,
      kind: span.span_type === "placeholder" ? "variable" : "element",
      sourceExpr: span.data,
    });
  }
  return out;
}

import { DirectionalText, cn, validateTags } from "@neokapi/ui-primitives";
import { useMemo } from "react";
import type { PluralForm, Run } from "@neokapi/kapi-format";
import type { BlockInfo } from "../../types/api";
import { blockHasInlineCodes, pluralForms, pluralOf, sourceRuns, targetRuns } from "./blockContent";
import { codeSpans } from "./editorText";
import { getTargetText } from "./blockStatus";
import { FormattedSourceDisplay } from "./FormattedSourceDisplay";
import { AlertTriangle } from "../icons";

/**
 * Collapsed-cell renderer for a target. Handles three shapes uniformly
 * so the grid and the review surfaces render targets identically:
 *
 *   1. Plural target (one plural run, or ICU text spelling one)
 *      → `other` form (or first present) rendered through
 *        FormattedSourceDisplay with its codes as chips, plus a
 *        "plural" badge so the row signals there are more forms behind the
 *        click-to-edit.
 *   2. Single target on a block with inline codes
 *      → chip rendering, with a warning when its codes differ from the
 *        source's.
 *   3. Plain target → text.
 *
 * Extracted from TranslationEditor so the shared TableView (and any future
 * row-based surface) renders targets identically.
 */
export function CollapsedTargetCell({
  block,
  locale,
  testId,
}: {
  block: BlockInfo;
  locale: string;
  testId: string;
}) {
  const target = useMemo(() => targetRuns(block, locale), [block, locale]);
  const plural = useMemo(() => pluralOf(target), [target]);

  if (plural) {
    const forms = pluralForms(plural);
    const available = Object.keys(forms) as PluralForm[];
    const shown: PluralForm = forms.other ? "other" : available[0];
    return (
      <DirectionalText
        locale={locale}
        className="text-foreground"
        data-testid={testId}
        data-plural-preview="true"
      >
        <FormattedSourceDisplay runs={forms[shown] ?? []} locale={locale} />
        <span
          className="ml-2 inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-xs uppercase tracking-wide text-muted-foreground"
          title={`Plural target: showing "${shown}" of ${available.length} form(s)`}
        >
          plural · {shown}
        </span>
      </DirectionalText>
    );
  }

  if (blockHasInlineCodes(block) && target.length > 0) {
    return (
      <DirectionalText locale={locale} className="text-foreground" data-testid={testId}>
        <FormattedSourceDisplay runs={target} locale={locale} />
        <RowTagWarning sourceRuns={sourceRuns(block)} targetRuns={target} />
      </DirectionalText>
    );
  }

  const rawTarget = getTargetText(block, locale);
  return (
    <DirectionalText
      locale={locale}
      className={cn(rawTarget ? "text-foreground" : "text-muted-foreground italic")}
      data-testid={testId}
    >
      {rawTarget || (block.translatable ? "Click to translate..." : "")}
    </DirectionalText>
  );
}

/** Warning icon when a target's inline codes differ from the source's. */
export function RowTagWarning({
  sourceRuns,
  targetRuns,
}: {
  sourceRuns: readonly Run[];
  targetRuns: readonly Run[];
}) {
  const validation = useMemo(
    () => validateTags(codeSpans(sourceRuns), codeSpans(targetRuns)),
    [sourceRuns, targetRuns],
  );

  if (validation.valid && validation.warnings.length === 0) return null;

  const issues = [...validation.errors, ...validation.warnings];
  const tooltip = issues.map((i) => i.message).join("\n");

  return (
    <span
      title={tooltip}
      data-testid="tag-warning"
      className={cn(
        "ml-1 cursor-help inline-flex",
        validation.errors.length > 0 ? "text-destructive" : "text-warning",
      )}
    >
      <AlertTriangle className="w-3.5 h-3.5" />
    </span>
  );
}

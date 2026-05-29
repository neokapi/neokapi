import { cn } from "@neokapi/ui-primitives";
import { useMemo } from "react";
import { validateTags, parsePluralFormForChips } from "@neokapi/ui-primitives";
import type { BlockInfo, SpanInfo } from "../../types/api";
import { FormattedSourceDisplay } from "./FormattedSourceDisplay";
import { AlertTriangle } from "../icons";

/**
 * Collapsed-cell renderer for a target. Handles three shapes uniformly
 * (AD #408 / #409):
 *
 *   1. Plural target (`targets[locale]` is ICU plural syntax)
 *      → render the `other` form's chips via FormattedSourceDisplay, with a
 *        "plural" badge so the row signals there are more forms behind the
 *        click-to-edit.
 *   2. Single target with inline codes (`has_spans` + `targets_coded`)
 *      → chip rendering.
 *   3. Plain target (`targets[locale]`) → text.
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
  const sourceSpans = block.source_spans ?? [];
  const rawTarget = block.targets[locale] ?? "";
  const codedTarget = block.targets_coded?.[locale] ?? "";

  // Plural takes priority — it lives in `targets[locale]` only.
  const pluralPreview = useMemo(
    () => (rawTarget ? parsePluralFormForChips(rawTarget, sourceSpans) : null),
    [rawTarget, sourceSpans],
  );

  if (pluralPreview) {
    return (
      <span className="text-foreground" data-testid={testId} data-plural-preview="true">
        <FormattedSourceDisplay codedText={pluralPreview.codedText} spans={pluralPreview.spans} />
        <span
          className="ml-2 inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-xs uppercase tracking-wide text-muted-foreground"
          title={`Plural target — showing "${pluralPreview.shownForm}" of ${pluralPreview.availableForms.length} form(s)`}
        >
          plural · {pluralPreview.shownForm}
        </span>
      </span>
    );
  }

  if (block.has_spans && codedTarget) {
    return (
      <span className="text-foreground" data-testid={testId}>
        <FormattedSourceDisplay codedText={codedTarget} spans={sourceSpans} />
        <RowTagWarning sourceSpans={sourceSpans} targetCodedText={codedTarget} />
      </span>
    );
  }

  return (
    <span
      className={cn(rawTarget ? "text-foreground" : "text-muted-foreground italic")}
      data-testid={testId}
    >
      {rawTarget || (block.translatable ? "Click to translate..." : "")}
    </span>
  );
}

/** Row-level validation indicator for tag mismatches. */
export function RowTagWarning({
  sourceSpans,
  targetCodedText,
}: {
  sourceSpans: SpanInfo[];
  targetCodedText: string;
}) {
  const targetSpans = useMemo(() => {
    const spans: SpanInfo[] = [];
    for (const ch of targetCodedText) {
      const code = ch.charCodeAt(0);
      if (code >= 0xe001 && code <= 0xe003) {
        if (spans.length < sourceSpans.length) {
          spans.push(sourceSpans[spans.length]);
        }
      }
    }
    return spans;
  }, [targetCodedText, sourceSpans]);

  const validation = useMemo(
    () => validateTags(sourceSpans, targetSpans),
    [sourceSpans, targetSpans],
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

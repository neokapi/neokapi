import {
  Button,
  getDefaultRegistry,
  tagColors,
  type SpanInfo,
  type TagColorScheme,
} from "@neokapi/ui-primitives";
import { useMemo } from "react";
import type { Run } from "@neokapi/kapi-format";
import { codeSpans } from "./editorText";

interface VisualEditorToolbarProps {
  /** The source, whose paired codes the toolbar offers. */
  sourceRuns: readonly Run[];
  /** Called when a code button is pressed; the open editor places the code. */
  onInsertTag: () => void;
  disabled?: boolean;
}

interface ToolbarEntry {
  typeName: string;
  span: SpanInfo;
  colors: TagColorScheme;
  label: string;
  shortcut: string;
}

/**
 * Compact formatting toolbar showing the unique paired-code types in the
 * source. Each button opens the target editor, where the code is inserted.
 */
export function VisualEditorToolbar({
  sourceRuns,
  onInsertTag,
  disabled,
}: VisualEditorToolbarProps) {
  const registry = getDefaultRegistry();

  const entries = useMemo(() => {
    const seen = new Set<string>();
    const result: ToolbarEntry[] = [];
    let shortcutIdx = 1;

    for (const span of codeSpans(sourceRuns)) {
      if (span.span_type !== "opening") continue;
      if (seen.has(span.type)) continue;
      seen.add(span.type);

      const info = registry.lookupOrFallback(span.type);
      result.push({
        typeName: span.type,
        span,
        colors: tagColors(span),
        label: info.label,
        shortcut: `Ctrl+${shortcutIdx}`,
      });
      shortcutIdx++;
    }

    return result;
  }, [sourceRuns, registry]);

  if (entries.length === 0) return null;

  return (
    <div className="flex items-center gap-px">
      {entries.map((entry) => (
        <Button
          key={entry.typeName}
          variant="ghost"
          size="sm"
          disabled={disabled}
          onClick={onInsertTag}
          title={`${entry.label} (${entry.shortcut})`}
          className="min-w-7"
          style={disabled ? undefined : { color: entry.colors.text }}
          data-testid={`toolbar-${entry.typeName}`}
        >
          {entry.label}
        </Button>
      ))}
    </div>
  );
}

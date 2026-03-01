import { useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { getDefaultRegistry } from "../../vocabularies";
import { Button } from "../ui/button";
import { tagColors, type TagColorScheme } from "./tagSemantics";

interface VisualEditorToolbarProps {
  sourceSpans: SpanInfo[];
  onInsertTag: (span: SpanInfo) => void;
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
 * Compact formatting toolbar showing unique tag types from source spans.
 * Each button inserts the corresponding opening span into the target editor.
 */
export function VisualEditorToolbar({
  sourceSpans,
  onInsertTag,
  disabled,
}: VisualEditorToolbarProps) {
  const registry = getDefaultRegistry();

  const entries = useMemo(() => {
    const seen = new Set<string>();
    const result: ToolbarEntry[] = [];
    let shortcutIdx = 1;

    for (const span of sourceSpans) {
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
  }, [sourceSpans, registry]);

  if (entries.length === 0) return null;

  return (
    <div style={toolbarStyle}>
      {entries.map((entry) => (
        <Button
          key={entry.typeName}
          variant="ghost"
          size="sm"
          disabled={disabled}
          onClick={() => onInsertTag(entry.span)}
          title={`${entry.label} (${entry.shortcut})`}
          style={{
            color: disabled ? undefined : entry.colors.text,
            minWidth: 28,
          }}
          data-testid={`toolbar-${entry.typeName}`}
        >
          {entry.label}
        </Button>
      ))}
    </div>
  );
}

const toolbarStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 1,
};

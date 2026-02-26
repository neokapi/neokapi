import { useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { Button } from "../ui/button";
import {
  semanticCategory,
  tagColors,
  tagNameFromData,
  type SemanticCategory,
  type TagColorScheme,
} from "./tagSemantics";

interface VisualEditorToolbarProps {
  sourceSpans: SpanInfo[];
  onInsertTag: (span: SpanInfo) => void;
  disabled?: boolean;
}

interface ToolbarEntry {
  category: SemanticCategory;
  span: SpanInfo;
  colors: TagColorScheme;
  label: string;
  shortcut: string;
}

const categoryLabel: Record<SemanticCategory, { text: string; style?: React.CSSProperties }> = {
  bold: { text: "B", style: { fontWeight: 700 } },
  italic: { text: "I", style: { fontStyle: "italic" } },
  underline: { text: "U", style: { textDecoration: "underline" } },
  strikethrough: { text: "S", style: { textDecoration: "line-through" } },
  link: { text: "\u{1F517}" },
  image: { text: "img" },
  code: { text: "</>", style: { fontFamily: "monospace", fontSize: 11 } },
  subscript: { text: "sub" },
  superscript: { text: "sup" },
  break: { text: "br" },
  generic: { text: "" },
};

function entryLabel(entry: ToolbarEntry): string {
  const def = categoryLabel[entry.category];
  if (def.text) return def.text;
  return tagNameFromData(entry.span.data) || "tag";
}

/**
 * Compact formatting toolbar showing unique tag categories from source spans.
 * Each button inserts the corresponding opening span into the target editor.
 */
export function VisualEditorToolbar({
  sourceSpans,
  onInsertTag,
  disabled,
}: VisualEditorToolbarProps) {
  const entries = useMemo(() => {
    const seen = new Set<SemanticCategory>();
    const result: ToolbarEntry[] = [];
    let shortcutIdx = 1;

    for (const span of sourceSpans) {
      if (span.span_type !== "opening") continue;
      const cat = semanticCategory(span);
      if (seen.has(cat)) continue;
      seen.add(cat);

      result.push({
        category: cat,
        span,
        colors: tagColors(span),
        label: "",
        shortcut: `Ctrl+${shortcutIdx}`,
      });
      shortcutIdx++;
    }

    // Fill in labels after construction
    for (const entry of result) {
      entry.label = entryLabel(entry);
    }

    return result;
  }, [sourceSpans]);

  if (entries.length === 0) return null;

  return (
    <div style={toolbarStyle}>
      {entries.map((entry) => {
        const labelDef = categoryLabel[entry.category];
        return (
          <Button
            key={entry.category}
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() => onInsertTag(entry.span)}
            title={`${entry.label} (${entry.shortcut})`}
            style={{
              color: disabled ? undefined : entry.colors.text,
              minWidth: 28,
              ...labelDef.style,
            }}
            data-testid={`toolbar-${entry.category}`}
          >
            {entry.label}
          </Button>
        );
      })}
    </div>
  );
}

const toolbarStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 1,
};

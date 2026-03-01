import { useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { parseCodedSegments } from "./codedText";
import {
  semanticCategory, semanticTooltip, tagColors,
  type SemanticCategory, type TagColorScheme,
} from "./tagSemantics";

interface FormattedSourceDisplayProps {
  codedText: string;
  spans: SpanInfo[];
}

/** CSS formatting per semantic category applied to enclosed text. */
const categoryStyle: Record<SemanticCategory, React.CSSProperties> = {
  bold: { fontWeight: 700 },
  italic: { fontStyle: "italic" },
  underline: { textDecoration: "underline" },
  strikethrough: { textDecoration: "line-through" },
  link: { textDecoration: "underline", color: "rgb(34, 197, 94)" },
  code: { fontFamily: "monospace", borderRadius: 2 },
  subscript: { verticalAlign: "sub", fontSize: "smaller" },
  superscript: { verticalAlign: "super", fontSize: "smaller" },
  image: {},
  break: {},
  generic: {},
};

/** Reduce tag bg opacity from ~0.15 to ~0.08 for a subtle tint. */
function tintBg(colors: TagColorScheme): string {
  return colors.bg.replace(/[\d.]+\)$/, "0.08)");
}

/** Slightly darker bg for code spans. */
function codeBg(colors: TagColorScheme): string {
  return colors.bg.replace(/[\d.]+\)$/, "0.2)");
}

interface StackEntry {
  span: SpanInfo;
  category: SemanticCategory;
  colors: TagColorScheme;
}

/**
 * Read-only display of source text with actual formatting applied.
 *
 * Instead of abstract tag chips, text is rendered with the visual
 * formatting each span implies (bold appears bold, links underlined, etc.)
 * plus a faint background tint in the category color.
 *
 * Placeholders (br, img) appear as small inline pills — matching
 * the TagChipComponent placeholder style.
 */
export function FormattedSourceDisplay({ codedText, spans }: FormattedSourceDisplayProps) {
  const segments = parseCodedSegments(codedText, spans);

  const rendered = useMemo(() => {
    const elements: React.ReactNode[] = [];
    const stack: StackEntry[] = [];

    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i];

      if (seg.type === "text") {
        // Build merged style from all currently-open spans
        const mergedStyle: React.CSSProperties = {};
        const textDecorations: string[] = [];

        for (const entry of stack) {
          const catStyle = categoryStyle[entry.category];
          Object.assign(mergedStyle, catStyle);
          // Collect text-decoration values for merging
          if (catStyle.textDecoration) {
            textDecorations.push(catStyle.textDecoration as string);
          }
        }

        if (textDecorations.length > 0) {
          mergedStyle.textDecoration = textDecorations.join(" ");
        }

        // Background tint from innermost span
        if (stack.length > 0) {
          const inner = stack[stack.length - 1];
          const bg = inner.category === "code"
            ? codeBg(inner.colors)
            : tintBg(inner.colors);
          mergedStyle.backgroundColor = bg;
          mergedStyle.borderRadius = 2;
          mergedStyle.padding = "0 1px";
        }

        // Tooltip from innermost span
        const tooltip = stack.length > 0
          ? semanticTooltip(stack[stack.length - 1].span)
          : undefined;

        elements.push(
          <span key={i} style={mergedStyle} title={tooltip}>
            {seg.value}
          </span>,
        );
      } else {
        // Tag segment
        const span = seg.spanInfo;
        const category = semanticCategory(span);
        const colors = tagColors(span);

        if (span.span_type === "opening") {
          stack.push({ span, category, colors });
          // No visible element — formatting applied to enclosed text
        } else if (span.span_type === "closing") {
          // Pop matching entry
          for (let j = stack.length - 1; j >= 0; j--) {
            if (stack[j].span.id === span.id) {
              stack.splice(j, 1);
              break;
            }
          }
          // No visible element
        } else {
          // Placeholder — render as small inline pill
          const label =
            category === "break" ? "br"
            : category === "image" ? "img"
            : span.type || "?";

          elements.push(
            <span
              key={i}
              style={{
                ...placeholderStyle,
                backgroundColor: colors.bg,
                borderColor: colors.border,
                color: colors.text,
              }}
              title={semanticTooltip(span)}
            >
              {label}
            </span>,
          );
        }
      }
    }

    return elements;
  }, [segments]);

  return <span>{rendered}</span>;
}

const placeholderStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  padding: "0 4px",
  margin: "0 1px",
  borderRadius: 3,
  border: "1px solid",
  fontSize: 10,
  fontFamily: "monospace",
  fontWeight: 500,
  lineHeight: "16px",
  verticalAlign: "middle",
  cursor: "default",
  userSelect: "none",
  whiteSpace: "nowrap",
};

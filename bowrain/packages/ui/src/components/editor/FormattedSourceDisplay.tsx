import { useMemo } from "react";
import type { EntityInfo, SpanInfo } from "../../types/api";
import {
  DirectionalText,
  getDefaultRegistry,
  parseCodedSegments,
  semanticLabel,
  semanticTooltip,
  tagColors,
  type TagColorScheme,
} from "@neokapi/ui-primitives";
import { entityMarks, markEntities } from "./entityMarks";

interface FormattedSourceDisplayProps {
  codedText: string;
  spans: SpanInfo[];
  /**
   * Entity annotations to underline, positioned over the plain text. Optional:
   * a target cell has none, and a source cell shows them only where the surface
   * has loaded them.
   */
  entities?: EntityInfo[];
  /**
   * The locale this text is written in (the source language, or the target
   * being displayed) — every caller should pass it: without it this reads
   * left-to-right regardless of the actual language.
   */
  locale?: string;
}

/** Map HTML open tags from the vocabulary to CSS styles for preview rendering. */
const htmlTagStyle: Record<string, React.CSSProperties> = {
  "<b>": { fontWeight: 700 },
  "<strong>": { fontWeight: 700 },
  "<i>": { fontStyle: "italic" },
  "<em>": { fontStyle: "italic" },
  "<u>": { textDecoration: "underline" },
  "<s>": { textDecoration: "line-through" },
  "<del>": { textDecoration: "line-through" },
  "<a>": { textDecoration: "underline", color: "rgb(34, 197, 94)" },
  "<code>": { fontFamily: "monospace", borderRadius: 2 },
  "<sub>": { verticalAlign: "sub", fontSize: "smaller" },
  "<sup>": { verticalAlign: "super", fontSize: "smaller" },
};

/** Derive CSS style from a span's vocabulary HTML rendering. */
function spanStyle(spanType: string): React.CSSProperties {
  const info = getDefaultRegistry().lookupOrFallback(spanType);
  const openTag = info.html.open;
  if (openTag) {
    // Normalize: extract just the tag name portion, e.g. '<b>' from '<b class="x">'
    const match = openTag.match(/^<(\w+)/);
    if (match) {
      return htmlTagStyle[`<${match[1]}>`] ?? {};
    }
  }
  return {};
}

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
  style: React.CSSProperties;
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
export function FormattedSourceDisplay({
  codedText,
  spans,
  entities,
  locale,
}: FormattedSourceDisplayProps) {
  const segments = useMemo(() => parseCodedSegments(codedText, spans), [codedText, spans]);

  // Entity positions are stated over the plain text — the text segments
  // concatenated, with the inline codes contributing nothing — so they are
  // resolved once here and applied as each text segment is emitted.
  const marks = useMemo(
    () =>
      entityMarks(
        segments.reduce((text, seg) => (seg.type === "text" ? text + seg.value : text), ""),
        entities,
      ),
    [segments, entities],
  );

  const rendered = useMemo(() => {
    const elements: React.ReactNode[] = [];
    const stack: StackEntry[] = [];
    // Running code-point offset into the plain text, for the entity marks.
    let plainOffset = 0;

    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i];

      if (seg.type === "text") {
        // Build merged style from all currently-open spans
        const mergedStyle: React.CSSProperties = {};
        const textDecorations: string[] = [];

        for (const entry of stack) {
          Object.assign(mergedStyle, entry.style);
          // Collect text-decoration values for merging
          if (entry.style.textDecoration) {
            textDecorations.push(entry.style.textDecoration as string);
          }
        }

        if (textDecorations.length > 0) {
          mergedStyle.textDecoration = textDecorations.join(" ");
        }

        // Background tint from innermost span
        if (stack.length > 0) {
          const inner = stack[stack.length - 1];
          const isCode = inner.style.fontFamily === "monospace";
          const bg = isCode ? codeBg(inner.colors) : tintBg(inner.colors);
          mergedStyle.backgroundColor = bg;
          mergedStyle.borderRadius = 2;
          mergedStyle.padding = "0 1px";
        }

        // Tooltip from innermost span
        const tooltip =
          stack.length > 0 ? semanticTooltip(stack[stack.length - 1].span) : undefined;

        const offset = plainOffset;
        plainOffset += Array.from(seg.value).length;

        elements.push(
          <span key={i} style={mergedStyle} title={tooltip}>
            {markEntities(seg.value, offset, marks)}
          </span>,
        );
      } else {
        // Tag segment
        const span = seg.spanInfo;
        const colors = tagColors(span);

        if (span.span_type === "opening") {
          stack.push({ span, style: spanStyle(span.type), colors });
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
        } else if (span.type === "struct:break") {
          // Line break — render as return symbol + actual break
          elements.push(
            <span key={i}>
              <span
                className="text-gray-500/80 text-xs select-none cursor-default"
                title="Line break"
              >
                {"\u23CE"}
              </span>
              <br />
            </span>,
          );
        } else {
          // Placeholder — render as small inline pill. The label comes from the
          // registry, which expands the vocabulary's template over the span, so
          // a variable chip names the variable rather than its type.
          const label = semanticLabel(span);

          elements.push(
            <span
              key={i}
              className="inline-flex items-center px-1 mx-px rounded-sm border text-[10px] font-mono font-medium leading-4 align-middle cursor-default select-none whitespace-nowrap"
              style={{
                backgroundColor: colors.bg,
                borderColor: colors.border,
                color: colors.text,
              }}
              title={semanticTooltip(span)}
              data-inline-code={span.span_type}
              dir="ltr"
            >
              {label}
            </span>,
          );
        }
      }
    }

    return elements;
  }, [segments, marks]);

  return <DirectionalText locale={locale}>{rendered}</DirectionalText>;
}

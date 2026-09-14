/**
 * Render plural targets the same way single-unit targets render.
 *
 * The grid's collapsed-cell renderer wants `(codedText, spans)` so it
 * can hand them to `FormattedSourceDisplay` and get styled chips. For
 * single-unit targets that's what `block.targets_coded[locale]` carries
 * directly. For plural targets, the canonical storage is an ICU string
 * in `block.targets[locale]` — this helper picks a representative form
 * out of that string and converts its content into the same coded
 * shape, so the cell preview stays visually consistent across plural
 * and non-plural rows.
 *
 * See AD #408 / #409.
 */

import type { Run } from "@neokapi/kapi-format";
import { parseICUPluralString, type PluralForm } from "@neokapi/kapi-format";

import type { SpanInfo } from "../../types/span";
import { runsToCoded } from "./runsCodedBridge";

export interface PluralCellPreview {
  /** Coded text + spans for `FormattedSourceDisplay`. */
  codedText: string;
  spans: SpanInfo[];
  /** The CLDR form whose content was rendered. */
  shownForm: PluralForm;
  /** All forms present on the target — useful for a "▾ N forms" badge. */
  availableForms: PluralForm[];
  /** Pivot variable name. */
  pivot: string;
}

/**
 * Parse an ICU plural string and return chip-rendering data for one
 * representative form. Returns `null` when:
 *   - The input isn't a plural construct (callers should fall back
 *     to plain-text display).
 *   - The plural has no forms at all (corrupt input).
 *
 * Form selection priority: caller's `preferredForm` if present →
 * `other` (CLDR fallback) → first form in the map.
 *
 * @param icuString    The raw target string (often `block.targets[locale]`).
 * @param sourceSpans  The block's source spans, used to resolve
 *                     `{=equiv}` / `{/=equiv}` / `{equiv}` markers
 *                     inside the form back to typed `SpanInfo` entries.
 * @param preferredForm Defaults to `'other'`.
 */
export function parsePluralFormForChips(
  icuString: string,
  sourceSpans: readonly SpanInfo[],
  preferredForm: PluralForm = "other",
): PluralCellPreview | null {
  const parsed = parseICUPluralString(icuString, (content) =>
    parseFormContentToRuns(content, sourceSpans),
  );
  if (!parsed || parsed.length === 0) return null;
  const wrapper = parsed[0];
  if (!("plural" in wrapper)) return null;

  const plural = wrapper.plural;
  const formNames = Object.keys(plural.forms) as PluralForm[];
  if (formNames.length === 0) return null;

  const shownForm: PluralForm =
    plural.forms[preferredForm] !== undefined
      ? preferredForm
      : plural.forms.other !== undefined
        ? "other"
        : formNames[0];

  const formRuns = plural.forms[shownForm] ?? [];
  const { codedText, spans } = runsToCoded(formRuns);

  return {
    codedText,
    spans,
    shownForm,
    availableForms: formNames,
    pivot: plural.pivot,
  };
}

// ─── Internals ───────────────────────────────────────────────────

/**
 * Parses a plural form's flat content into a Run sequence. Used as
 * the `parseContent` hook for `parseICUPluralString`. Recognises the
 * marker grammar `flattenRuns` emits inside form bodies:
 *
 *   `{=equiv}`  → PcOpenRun  (resolved via sourceSpans by equiv)
 *   `{/=equiv}` → PcCloseRun
 *   `{equiv}`   → PlaceholderRun
 *
 * Tokens that don't match a source span fall through as literal text
 * — keeps the renderer honest about whatever is actually stored.
 */
function parseFormContentToRuns(content: string, sourceSpans: readonly SpanInfo[]): Run[] {
  const out: Run[] = [];
  let buffer = "";
  let i = 0;

  const flush = () => {
    if (buffer.length > 0) {
      out.push({ text: buffer });
      buffer = "";
    }
  };

  while (i < content.length) {
    if (content[i] !== "{") {
      buffer += content[i];
      i++;
      continue;
    }
    const end = content.indexOf("}", i);
    if (end < 0) {
      buffer += content.slice(i);
      break;
    }
    const inner = content.slice(i + 1, end);
    const run = resolveMarker(inner, sourceSpans);
    if (run) {
      flush();
      out.push(run);
    } else {
      // Unknown marker — preserve as literal so the cell shows the
      // truth instead of silently dropping content.
      buffer += content.slice(i, end + 1);
    }
    i = end + 1;
  }
  flush();
  return out;
}

function resolveMarker(inner: string, sourceSpans: readonly SpanInfo[]): Run | null {
  if (inner.startsWith("/=")) {
    const equiv = inner.slice(2);
    const span = sourceSpans.find((s) => s.span_type === "closing" && s.equiv_text === equiv);
    if (span) {
      return {
        pcClose: {
          id: span.id,
          type: span.type,
          ...(span.sub_type ? { subType: span.sub_type } : {}),
          data: span.data,
          equiv,
        },
      };
    }
    return null;
  }
  if (inner.startsWith("=")) {
    const equiv = inner.slice(1);
    const span = sourceSpans.find((s) => s.span_type === "opening" && s.equiv_text === equiv);
    if (span) {
      return {
        pcOpen: {
          id: span.id,
          type: span.type,
          ...(span.sub_type ? { subType: span.sub_type } : {}),
          data: span.data,
          equiv,
          ...(span.display_text ? { disp: span.display_text } : {}),
        },
      };
    }
    return null;
  }

  // Plain `{name}` — placeholder lookup.
  const span = sourceSpans.find((s) => s.span_type === "placeholder" && s.equiv_text === inner);
  if (span) {
    return {
      ph: {
        id: span.id,
        type: span.type,
        ...(span.sub_type ? { subType: span.sub_type } : {}),
        data: span.data,
        equiv: inner,
        ...(span.display_text ? { disp: span.display_text } : {}),
      },
    };
  }
  return null;
}

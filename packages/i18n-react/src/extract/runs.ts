/**
 * Converts a JSX element's children into a `Run[]` sequence plus the
 * flat text template the transform plugin's `hashKey` input expects.
 *
 * Every child maps onto one of the kapi-format Run kinds:
 *
 *   JSXText                                   → TextRun
 *   JSXExpressionContainer (plain)            → PlaceholderRun ("jsx:var")
 *   JSXExpressionContainer with JSX inside    → PlaceholderRun ("jsx:node", optional)
 *   JSXElement, no children                   → PlaceholderRun ("jsx:element")
 *   JSXElement, translate="no"                → PlaceholderRun ("jsx:element")
 *   JSXElement, has children                  → PcOpenRun + inner runs + PcCloseRun ("jsx:element")
 *
 * The paired form keeps inline elements like `<a>here</a>` and
 * `<strong>{count}</strong>` inside their parent's translatable
 * sentence; the runtime cloneElements the wrapping element with the
 * inner runs as its children at render time. The flat template is
 * symmetric: paired open is `{=mN}`, close is `{/=mN}`, standalone
 * is `{=mN}` with no matching close in the same scope. Both extract
 * and `plugin/transform.ts` share the same `=mN` numbering so hash
 * inputs line up.
 *
 * Whitespace handling: raw JSX text collapses to single spaces, the
 * outer run sequence trims at the edges, and purely-whitespace text
 * between structural runs is preserved so the translator sees
 * `"Save {=m0}"` with its padding.
 *
 * The builder also records a `Placeholder` entry per unique name
 * (`equiv`) so the Block carries the metadata validators and CAT
 * tools rely on.
 */

import type { Expression, JSXElement, JSXElementChild, JSXExpressionContainer } from "@swc/core";

import type {
  PcCloseRun,
  PcOpenRun,
  Placeholder,
  PlaceholderRun,
  PluralRunWrapper,
  Run,
  SelectRunWrapper,
  TextRun,
} from "@neokapi/kapi-format";

import { hasICUSyntax } from "../runtime/icu.ts";
import {
  containsJSX,
  dedupName,
  exprToName,
  getStringAttr,
  getTagName,
  resolveHTMLElement,
} from "./ast.ts";
import {
  isPluralElement,
  isSelectElement,
  parsePlural,
  parseSelect,
  type PluralFormKey,
  type PluralInfo,
  type SelectInfo,
} from "./plural.ts";

export interface BuildRunsOptions {
  componentMap: Record<string, string>;
  /**
   * Lookup that returns the UTF-8 slice of source for a span. SWC
   * spans are byte offsets; the extractor owns the raw source
   * string and a base-offset converter, so the slicing lives up
   * there and runs.ts stays source-string-free.
   */
  sourceSlice(start: number, end: number): string;
}

/**
 * One template-token occurrence with its raw SWC byte spans. The
 * transform maps these through its offset converter to splice source
 * text; spans are recorded verbatim from the AST (same domain the
 * caller's `sourceSlice` receives).
 *
 * Unlike `placeholders` (deduped by name for Block metadata), this
 * list has one entry per appearance, in template order — the shape
 * the transform's params/elements objects are built from.
 */
export interface Occurrence {
  name: string;
  kind: "var" | "node" | "element" | "paired" | "pivot";
  /** Span of the bare expression (or the whole element). */
  exprStart: number;
  exprEnd: number;
  /** Span including the `{…}` container (or the whole element). */
  fullStart: number;
  fullEnd: number;
  /** Paired elements only: opening / closing tag spans. */
  openStart?: number;
  openEnd?: number;
  closeStart?: number;
  closeEnd?: number;
}

export interface BuildRunsResult {
  runs: Run[];
  /**
   * The flat text used as the hash input — text verbatim, expressions
   * as `{name}`, inline elements as `{=mN}`. Bytes match what
   * `plugin/transform.ts` feeds to `hashKey`.
   */
  flatText: string;
  placeholders: Placeholder[];
  /** Per-appearance token spans, in template order. */
  occurrences: Occurrence[];
}

interface BuilderState {
  runs: Run[];
  flatText: string;
  /** Unique ids within the current runs scope, feeds Run.id. */
  idSeq: number;
  /** equivs already reserved in this scope (param dedup). */
  usedNames: Set<string>;
  /** dedup keyed by placeholder name for the metadata table. */
  placeholders: Map<string, Placeholder>;
  occurrences: Occurrence[];
  componentMap: Record<string, string>;
  sourceSlice: BuildRunsOptions["sourceSlice"];
}

/**
 * Public entry: walk a translatable JSX element's children and emit
 * runs + the flat text template. Call once per Block. Accepts a bare
 * children array too, so JSX fragments (`<>…</>`) reuse the same
 * builder.
 */
export function buildRuns(
  el: JSXElement | { children?: readonly JSXElementChild[] },
  opts: BuildRunsOptions,
): BuildRunsResult {
  const state: BuilderState = {
    runs: [],
    flatText: "",
    idSeq: 0,
    usedNames: new Set(),
    placeholders: new Map(),
    occurrences: [],
    componentMap: opts.componentMap,
    sourceSlice: opts.sourceSlice,
  };
  walkChildren(el.children ?? [], state);
  return {
    runs: trimEdgeWhitespace(state.runs),
    flatText: state.flatText.trim(),
    placeholders: Array.from(state.placeholders.values()),
    occurrences: state.occurrences,
  };
}

function walkChildren(children: readonly JSXElementChild[], state: BuilderState): void {
  for (const child of children) {
    if (child.type === "JSXText") {
      appendText(state, child.value.replace(/\s+/g, " "));
    } else if (child.type === "JSXExpressionContainer") {
      appendExpression(state, child);
    } else if (child.type === "JSXElement") {
      appendJsxElement(state, child);
    }
    // JSXFragment and JSXSpreadChild are disallowed upstream by
    // isAllInlineContent; if they ever slip through we simply skip.
  }
}

// ─── Per-child emitters ───────────────────────────────────────────

function appendText(state: BuilderState, text: string): void {
  if (text.length === 0) return;
  // Coalesce adjacent text runs so a chunked ABI doesn't produce
  // visually-identical neighbours.
  const last = state.runs[state.runs.length - 1];
  if (last && "text" in last) {
    last.text += text;
  } else {
    const run: { text: string } = { text };
    state.runs.push(run as Run);
  }
  state.flatText += text;
}

function appendExpression(state: BuilderState, node: JSXExpressionContainer): void {
  if (node.expression.type === "JSXEmptyExpression") return;

  const id = nextId(state);
  const expr = node.expression;

  const src = spanSlice(expr, state);

  const exprSpan = (expr as { span?: { start: number; end: number } }).span;

  if (containsJSX(expr)) {
    // {cond && <X/>} / {cond ? <A/> : <B/>} — optional node. Equivs
    // get synthesized like the transform side so hash inputs line up.
    const equiv = dedupName(`=m${state.idSeq - 1}`, state.usedNames);
    state.runs.push({
      ph: {
        id,
        type: "jsx:node",
        data: src,
        equiv,
      },
    } satisfies PlaceholderRun);
    state.flatText += `{${equiv}}`;
    recordPlaceholder(state, {
      name: equiv,
      kind: "node",
      jsType: "ReactNode",
      sourceExpr: src,
      optional: true,
    });
    if (exprSpan) {
      state.occurrences.push({
        name: equiv,
        kind: "node",
        exprStart: exprSpan.start,
        exprEnd: exprSpan.end,
        fullStart: exprSpan.start,
        fullEnd: exprSpan.end,
      });
    }
    return;
  }

  const rawName = exprToName(expr as Expression);
  const equiv = dedupName(rawName, state.usedNames);
  state.runs.push({
    ph: {
      id,
      type: "jsx:var",
      data: `{${equiv}}`,
      equiv,
    },
  } satisfies PlaceholderRun);
  state.flatText += `{${equiv}}`;
  recordPlaceholder(state, {
    name: equiv,
    kind: "variable",
    sourceExpr: src,
  });
  if (exprSpan) {
    state.occurrences.push({
      name: equiv,
      kind: "var",
      exprStart: exprSpan.start,
      exprEnd: exprSpan.end,
      fullStart: node.span.start,
      fullEnd: node.span.end,
    });
  }
}

function appendJsxElement(state: BuilderState, el: JSXElement): void {
  const tag = getTagName(el);
  if (!tag) return;

  // W3C `translate="no"` marks an untranslatable island. Inside a
  // translatable parent the element travels whole, as an opaque
  // standalone placeholder: a file path, an identifier or a code
  // sample stays out of the message, and the runtime substitutes the
  // element exactly as the author wrote it. `hasTranslatableText`
  // reads the same attribute, so a parent whose only text sits in
  // such a child is never promoted in the first place.
  const opaque = getStringAttr(el, "translate") === "no";

  // Structured plural / select components get special handling —
  // the children carry form contents that must become typed Run[]
  // inside a PluralRun / SelectRun. Everything else falls through
  // to the default `jsx:element` placeholder.
  if (!opaque) {
    if (isPluralElement(el)) {
      const info = parsePlural(el);
      if (info) {
        appendPluralRun(state, el, info);
        return;
      }
    } else if (isSelectElement(el)) {
      const info = parseSelect(el);
      if (info) {
        appendSelectRun(state, el, info);
        return;
      }
    }
  }

  const resolved = resolveHTMLElement(tag, state.componentMap);
  const subType = resolved ?? tag;
  const children = el.children ?? [];

  // Zero children → standalone PlaceholderRun (icons, <br/>, empty
  // self-closing components), and so is a `translate="no"` island
  // whatever it holds. The runtime resolves these by looking up
  // `elements["=m<N>"]` and substituting directly — no inner content
  // to wrap.
  if (children.length === 0 || opaque) {
    const src = state.sourceSlice(el.span.start, el.span.end);
    const id = nextId(state);
    const equiv = dedupName(`=m${state.idSeq - 1}`, state.usedNames);
    state.runs.push({
      ph: {
        id,
        type: "jsx:element",
        subType,
        data: src,
        equiv,
      },
    } satisfies PlaceholderRun);
    state.flatText += `{${equiv}}`;
    recordPlaceholder(state, {
      name: equiv,
      kind: "element",
      jsType: "ReactNode",
      sourceExpr: src,
    });
    state.occurrences.push({
      name: equiv,
      kind: "element",
      exprStart: el.span.start,
      exprEnd: el.span.end,
      fullStart: el.span.start,
      fullEnd: el.span.end,
    });
    return;
  }

  // Has children → PcOpenRun + descend inner runs + PcCloseRun. The
  // wrapping element stays in `elements["=m<N>"]`; the runtime clones
  // it with the inner content as children at render time. Inner
  // markup may contain nested paired pairs, expression placeholders,
  // and text runs — all produced by recursing through walkChildren.
  const wholeSrc = state.sourceSlice(el.span.start, el.span.end);
  const openSrc = state.sourceSlice(el.opening.span.start, el.opening.span.end);
  const closeSrc = el.closing ? state.sourceSlice(el.closing.span.start, el.closing.span.end) : "";
  nextId(state);
  const ord = state.idSeq - 1;
  const equiv = dedupName(`=m${ord}`, state.usedNames);
  const idDigit = String(ord);

  state.runs.push({
    pcOpen: {
      id: idDigit,
      type: "jsx:element",
      subType,
      data: openSrc,
      equiv,
    },
  } satisfies PcOpenRun);
  state.flatText += `{${equiv}}`;

  walkChildren(children, state);

  state.runs.push({
    pcClose: {
      id: idDigit,
      type: "jsx:element",
      subType,
      data: closeSrc,
      equiv,
    },
  } satisfies PcCloseRun);
  state.flatText += `{/${equiv}}`;

  recordPlaceholder(state, {
    name: equiv,
    kind: "element",
    jsType: "ReactNode",
    sourceExpr: wholeSrc,
  });
  state.occurrences.push({
    name: equiv,
    kind: "paired",
    exprStart: el.span.start,
    exprEnd: el.span.end,
    fullStart: el.span.start,
    fullEnd: el.span.end,
    openStart: el.opening.span.start,
    openEnd: el.opening.span.end,
    closeStart: el.closing?.span.start,
    closeEnd: el.closing?.span.end,
  });
}

// ─── Plural / Select ─────────────────────────────────────────────

function appendPluralRun(state: BuilderState, el: JSXElement, info: PluralInfo): void {
  // Pivot is NOT added to usedNames: form bodies often reference the
  // pivot variable (`<Other>{count} items</Other>`), and we want that
  // `{count}` token to resolve to the pivot, not a deduped `count_2`.
  const pivotEquiv = info.pivotName;
  recordPlaceholder(state, {
    name: pivotEquiv,
    kind: "icu-pivot",
    jsType: "number",
    sourceExpr: pivotSourceFromEl(state, el, info.pivotSource),
  });
  recordPivotOccurrence(state, el, "count", pivotEquiv);

  const forms: Partial<Record<PluralFormKey, Run[]>> = {};
  const formFlat = new Map<PluralFormKey, string>();
  for (const { key, el: formEl } of info.forms) {
    const { runs: formRuns, flatText } = buildNestedFormRuns(state, formEl.children ?? []);
    forms[key] = formRuns;
    formFlat.set(key, flatText);
  }

  state.runs.push({
    plural: { pivot: pivotEquiv, forms },
  } satisfies PluralRunWrapper);
  state.flatText += icuPluralTemplate(
    pivotEquiv,
    info.forms.map((f) => f.key),
    formFlat,
  );
}

function appendSelectRun(state: BuilderState, el: JSXElement, info: SelectInfo): void {
  const pivotEquiv = info.pivotName;
  recordPlaceholder(state, {
    name: pivotEquiv,
    kind: "icu-pivot",
    jsType: "string",
    sourceExpr: pivotSourceFromEl(state, el, info.pivotSource),
  });
  recordPivotOccurrence(state, el, "value", pivotEquiv);

  const cases: Record<string, Run[]> = {};
  const caseFlat = new Map<string, string>();
  for (const { key, el: caseEl } of info.cases) {
    const { runs, flatText } = buildNestedFormRuns(state, caseEl.children ?? []);
    cases[key] = runs;
    caseFlat.set(key, flatText);
  }
  if (info.otherEl) {
    const { runs, flatText } = buildNestedFormRuns(state, info.otherEl.children ?? []);
    cases.other = runs;
    caseFlat.set("other", flatText);
  }

  state.runs.push({
    select: { pivot: pivotEquiv, cases },
  } satisfies SelectRunWrapper);
  state.flatText += icuSelectTemplate(pivotEquiv, Array.from(caseFlat.keys()), caseFlat);
}

/**
 * Runs the form's children through the same builder so inline
 * elements, expression containers, and text inside a form are
 * typed runs — `<strong>3</strong>` inside `<Other>` becomes a
 * `jsx:element` ph, not a literal string. The `idSeq` and
 * `usedNames` caches carry across forms so `=mN` tokens stay
 * globally unique within the block (hash input invariant).
 */
function buildNestedFormRuns(
  state: BuilderState,
  children: readonly import("@swc/core").JSXElementChild[],
): { runs: Run[]; flatText: string } {
  const savedRuns = state.runs;
  const savedFlat = state.flatText;
  state.runs = [];
  state.flatText = "";
  walkChildren(children, state);
  const runs = trimEdgeWhitespace(state.runs);
  const flatText = state.flatText.trim();
  state.runs = savedRuns;
  state.flatText = savedFlat;
  return { runs, flatText };
}

/**
 * Records the pivot prop of a `<Plural>` / `<Select>` as a param
 * occurrence so the transform emits `{ count: items.length }` in the
 * runtime call and `resolveICU` can evaluate the rule. Mirrors the
 * placeholder registration: NOT deduped against usedNames (form
 * bodies referencing the pivot share the name by design).
 */
function recordPivotOccurrence(
  state: BuilderState,
  el: JSXElement,
  propName: "count" | "value",
  pivotName: string,
): void {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== "JSXAttribute" || attr.name.type !== "Identifier") continue;
    if (attr.name.value !== propName) continue;
    const value = attr.value;
    if (!value) return;
    let span: { start: number; end: number } | undefined;
    if (value.type === "JSXExpressionContainer") {
      span = (value.expression as { span?: { start: number; end: number } }).span;
    } else if (value.type === "StringLiteral") {
      span = value.span;
    }
    if (!span) return;
    state.occurrences.push({
      name: pivotName,
      kind: "pivot",
      exprStart: span.start,
      exprEnd: span.end,
      fullStart: span.start,
      fullEnd: span.end,
    });
    return;
  }
}

function pivotSourceFromEl(state: BuilderState, el: JSXElement, fallback: string): string {
  // Walk opening attributes to find the pivot attr's expression span.
  // Falls back to the fallback source if the attribute layout is unusual.
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== "JSXAttribute" || attr.name.type !== "Identifier") continue;
    const name = attr.name.value;
    if (name !== "count" && name !== "value") continue;
    const value = attr.value;
    if (!value) continue;
    if (value.type === "JSXExpressionContainer") {
      return spanSlice(value.expression, state);
    }
    if (value.type === "StringLiteral") {
      return state.sourceSlice(value.span.start, value.span.end);
    }
  }
  return fallback;
}

function icuPluralTemplate(
  pivot: string,
  order: readonly PluralFormKey[],
  flat: Map<PluralFormKey, string>,
): string {
  const parts: string[] = [];
  for (const key of order) parts.push(`${key} {${flat.get(key) ?? ""}}`);
  return `{${pivot}, plural, ${parts.join(" ")}}`;
}

function icuSelectTemplate(
  pivot: string,
  order: readonly string[],
  flat: Map<string, string>,
): string {
  const parts: string[] = [];
  for (const key of order) parts.push(`${key} {${flat.get(key) ?? ""}}`);
  return `{${pivot}, select, ${parts.join(" ")}}`;
}

// ─── Helpers ──────────────────────────────────────────────────────

function nextId(state: BuilderState): string {
  state.idSeq++;
  return String(state.idSeq);
}

function recordPlaceholder(state: BuilderState, placeholder: Placeholder): void {
  if (state.placeholders.has(placeholder.name)) return;
  state.placeholders.set(placeholder.name, placeholder);
}

/**
 * Returns the raw source text of an AST node with a `span`, or ""
 * for nodes without span metadata (e.g. `JSXEmptyExpression`).
 */
function spanSlice(node: unknown, state: BuilderState): string {
  const span = (node as { span?: { start: number; end: number } }).span;
  if (!span) return "";
  return state.sourceSlice(span.start, span.end);
}

/**
 * Trim leading / trailing purely-whitespace text runs. Whitespace
 * between structural runs stays.
 */
function trimEdgeWhitespace(runs: Run[]): Run[] {
  if (runs.length === 0) return runs;
  const trimmed = [...runs];

  const first = trimmed[0];
  if (first && "text" in first) {
    const text = (first as TextRun).text.replace(/^\s+/, "");
    if (text === "") trimmed.shift();
    else (first as TextRun).text = text;
  }

  const last = trimmed[trimmed.length - 1];
  if (last && "text" in last) {
    const text = (last as TextRun).text.replace(/\s+$/, "");
    if (text === "") trimmed.pop();
    else (last as TextRun).text = text;
  }

  return trimmed;
}

// ─── t() argument runs ────────────────────────────────────────────

/**
 * A `t()` argument token: a brace around a member path and nothing else.
 *
 * Deliberately exact. The runtime substitutes by literal string match
 * (`out.replaceAll("{" + key + "}", …)`), so a token is one only if it is
 * spelled the way a key is spelled — `{ name }` with spaces around it
 * substitutes nothing at render time, and lifting it into a run would quietly
 * repair it, because a `ph` flattens back as `{equiv}`. A brace holding
 * anything else is left as the text it is.
 */
const T_ARGUMENT = String.raw`\{([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*)\}`;

/** What {@link tCallRuns} produces: the runs, and the metadata table for them. */
export interface TCallRunsResult {
  runs: Run[];
  placeholders: Placeholder[];
}

/**
 * The runs of a `t("…")` source string, with its arguments lifted out of the
 * text as placeholder runs.
 *
 * A `t()` string has no AST to walk — its arguments are brace tokens the
 * runtime substitutes by literal match. So until they were lifted they were
 * ordinary characters in an ordinary text run, and every mechanism that
 * protects a placeholder was reading a block that declared none: the editor
 * drew `use {replacement}` as prose with nothing to hold onto, an AI pass got a
 * sentence in which `{replacement}` was a word like any other, and
 * `DiffRunCodes` — the comparison that catches a target which dropped an inline
 * code — had no code to compare. A translation that reworded the token still
 * rendered. It rendered `{replacement}`, literally, to the reader.
 *
 * Lifting them puts a `t()` argument on the footing a JSX one has always had.
 * `jsx:var` is reused rather than a new vocabulary key invented: that entry is
 * the *variable* rendering — chip labelled with the equiv, not deletable — and
 * what differs between `<p>{name}</p>` and `t("{name}")` is where the extractor
 * found the token, which is not something a reviewer's chip should say.
 *
 * The flattened form is unchanged: `flattenRuns` writes a `ph` back as
 * `{equiv}`, so the compiled dictionary and the runtime lookup are byte-identical
 * to what the single text run produced. The block hash is unaffected too — it is
 * taken over the raw `t()` argument, never over the runs.
 *
 * ICU picker messages are left whole. Their braces belong to ICU, not to the
 * substitution pass: the argument, the categories and the `#` are one structure
 * `resolveICU` parses at render time, and placeholder-check already compares
 * that structure argument by argument rather than token by token
 * (core/tools/placeholders.go). Splitting a picker across runs would hand both
 * of them half a message.
 */
export function tCallRuns(text: string): TCallRunsResult {
  if (text === "") return { runs: [], placeholders: [] };
  if (hasICUSyntax(text)) return { runs: [{ text } as TextRun], placeholders: [] };

  const runs: Run[] = [];
  const placeholders = new Map<string, Placeholder>();
  let cursor = 0;

  for (const m of text.matchAll(new RegExp(T_ARGUMENT, "g"))) {
    const name = m[1];
    const at = m.index;
    if (at > cursor) runs.push({ text: text.slice(cursor, at) } as TextRun);
    runs.push({
      ph: {
        id: String(runs.length + 1),
        type: "jsx:var",
        data: m[0],
        equiv: name,
      },
    } satisfies PlaceholderRun);
    if (!placeholders.has(name)) {
      placeholders.set(name, { name, kind: "variable", sourceExpr: name });
    }
    cursor = at + m[0].length;
  }

  if (placeholders.size === 0) return { runs: [{ text } as TextRun], placeholders: [] };
  if (cursor < text.length) runs.push({ text: text.slice(cursor) } as TextRun);

  return { runs, placeholders: Array.from(placeholders.values()) };
}

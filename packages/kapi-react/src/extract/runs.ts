/**
 * Converts a JSX element's children into a `Run[]` sequence plus the
 * flat text template the transform plugin's `hashKey` input expects.
 *
 * Every child maps to exactly one run type:
 *
 *   JSXText                                   → TextRun
 *   JSXExpressionContainer (plain)            → PlaceholderRun ("jsx:var")
 *   JSXExpressionContainer with JSX inside    → PlaceholderRun ("jsx:node", optional)
 *   JSXElement (inline, any children)         → PlaceholderRun ("jsx:element")
 *
 * Inline elements with children collapse to one `ph` placeholder so
 * the flat template matches what `plugin/transform.ts` feeds to
 * `hashKey` — otherwise extract- and transform-side hashes would
 * drift and the runtime dict would stop resolving. The children of
 * an inline element become their own translatable Block through the
 * walker's normal descent, not nested runs under this one.
 *
 * Whitespace handling follows the legacy extractor: raw JSX text is
 * collapsed to single spaces, the final run sequence is trimmed at
 * the edges, and purely-whitespace text between structural runs is
 * preserved so the translator sees `"Save {=m0}"` with its padding.
 *
 * The builder also records a `Placeholder` entry per unique name
 * (`equiv`) so the Block carries the metadata validators and CAT
 * tools rely on.
 */

import type {
  Expression,
  JSXElement,
  JSXElementChild,
  JSXExpressionContainer,
} from '@swc/core';

import type {
  Placeholder,
  PlaceholderRun,
  PluralRunWrapper,
  Run,
  SelectRunWrapper,
  TextRun,
} from '@neokapi/kapi-format';

import {
  containsJSX,
  dedupName,
  exprToName,
  getTagName,
  resolveHTMLElement,
} from './ast.ts';
import {
  isPluralTag,
  isSelectTag,
  parsePlural,
  parseSelect,
  type PluralFormKey,
  type PluralInfo,
  type SelectInfo,
} from './plural.ts';

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

export interface BuildRunsResult {
  runs: Run[];
  /**
   * The flat text used as the hash input — text verbatim, expressions
   * as `{name}`, inline elements as `{=mN}`. Bytes match what
   * `plugin/transform.ts` feeds to `hashKey`.
   */
  flatText: string;
  placeholders: Placeholder[];
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
  componentMap: Record<string, string>;
  sourceSlice: BuildRunsOptions['sourceSlice'];
}

/**
 * Public entry: walk a translatable JSX element's children and emit
 * runs + the flat text template. Call once per Block.
 */
export function buildRuns(el: JSXElement, opts: BuildRunsOptions): BuildRunsResult {
  const state: BuilderState = {
    runs: [],
    flatText: '',
    idSeq: 0,
    usedNames: new Set(),
    placeholders: new Map(),
    componentMap: opts.componentMap,
    sourceSlice: opts.sourceSlice,
  };
  walkChildren(el.children ?? [], state);
  return {
    runs: trimEdgeWhitespace(state.runs),
    flatText: state.flatText.trim(),
    placeholders: Array.from(state.placeholders.values()),
  };
}

function walkChildren(children: readonly JSXElementChild[], state: BuilderState): void {
  for (const child of children) {
    if (child.type === 'JSXText') {
      appendText(state, child.value.replace(/\s+/g, ' '));
    } else if (child.type === 'JSXExpressionContainer') {
      appendExpression(state, child);
    } else if (child.type === 'JSXElement') {
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
  if (last && 'text' in last) {
    last.text += text;
  } else {
    const run: { text: string } = { text };
    state.runs.push(run as Run);
  }
  state.flatText += text;
}

function appendExpression(state: BuilderState, node: JSXExpressionContainer): void {
  if (node.expression.type === 'JSXEmptyExpression') return;

  const id = nextId(state);
  const expr = node.expression;

  const src = spanSlice(expr, state);

  if (containsJSX(expr)) {
    // {cond && <X/>} / {cond ? <A/> : <B/>} — optional node. Equivs
    // get synthesized like the transform side so hash inputs line up.
    const equiv = dedupName(`=m${state.idSeq - 1}`, state.usedNames);
    state.runs.push({
      ph: {
        id,
        type: 'jsx:node',
        data: src,
        equiv,
      },
    } satisfies PlaceholderRun);
    state.flatText += `{${equiv}}`;
    recordPlaceholder(state, {
      name: equiv,
      kind: 'node',
      jsType: 'ReactNode',
      sourceExpr: src,
      optional: true,
    });
    return;
  }

  const rawName = exprToName(expr as Expression);
  const equiv = dedupName(rawName, state.usedNames);
  state.runs.push({
    ph: {
      id,
      type: 'jsx:var',
      data: `{${equiv}}`,
      equiv,
    },
  } satisfies PlaceholderRun);
  state.flatText += `{${equiv}}`;
  recordPlaceholder(state, {
    name: equiv,
    kind: 'variable',
    sourceExpr: src,
  });
}

function appendJsxElement(state: BuilderState, el: JSXElement): void {
  const tag = getTagName(el);
  if (!tag) return;

  // Structured plural / select components get special handling —
  // the children carry form contents that must become typed Run[]
  // inside a PluralRun / SelectRun. Everything else falls through
  // to the default `jsx:element` placeholder.
  if (isPluralTag(tag)) {
    const info = parsePlural(el);
    if (info) {
      appendPluralRun(state, el, info);
      return;
    }
  } else if (isSelectTag(tag)) {
    const info = parseSelect(el);
    if (info) {
      appendSelectRun(state, el, info);
      return;
    }
  }

  const resolved = resolveHTMLElement(tag, state.componentMap);
  const subType = resolved ?? tag;

  const src = state.sourceSlice(el.span.start, el.span.end);
  const id = nextId(state);
  const equiv = dedupName(`=m${state.idSeq - 1}`, state.usedNames);
  state.runs.push({
    ph: {
      id,
      type: 'jsx:element',
      subType,
      data: src,
      equiv,
    },
  } satisfies PlaceholderRun);
  state.flatText += `{${equiv}}`;
  recordPlaceholder(state, {
    name: equiv,
    kind: 'element',
    jsType: 'ReactNode',
    sourceExpr: src,
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
    kind: 'icu-pivot',
    jsType: 'number',
    sourceExpr: pivotSourceFromEl(state, el, info.pivotSource),
  });

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
  state.flatText += icuPluralTemplate(pivotEquiv, info.forms.map((f) => f.key), formFlat);
}

function appendSelectRun(state: BuilderState, el: JSXElement, info: SelectInfo): void {
  const pivotEquiv = info.pivotName;
  recordPlaceholder(state, {
    name: pivotEquiv,
    kind: 'icu-pivot',
    jsType: 'string',
    sourceExpr: pivotSourceFromEl(state, el, info.pivotSource),
  });

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
    caseFlat.set('other', flatText);
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
  children: readonly import('@swc/core').JSXElementChild[],
): { runs: Run[]; flatText: string } {
  const savedRuns = state.runs;
  const savedFlat = state.flatText;
  state.runs = [];
  state.flatText = '';
  walkChildren(children, state);
  const runs = trimEdgeWhitespace(state.runs);
  const flatText = state.flatText.trim();
  state.runs = savedRuns;
  state.flatText = savedFlat;
  return { runs, flatText };
}

function pivotSourceFromEl(state: BuilderState, el: JSXElement, fallback: string): string {
  // Walk opening attributes to find the pivot attr's expression span.
  // Falls back to the fallback source if the attribute layout is unusual.
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    const name = attr.name.value;
    if (name !== 'count' && name !== 'value') continue;
    const value = attr.value;
    if (!value) continue;
    if (value.type === 'JSXExpressionContainer') {
      return spanSlice(value.expression, state);
    }
    if (value.type === 'StringLiteral') {
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
  for (const key of order) parts.push(`${key} {${flat.get(key) ?? ''}}`);
  return `{${pivot}, plural, ${parts.join(' ')}}`;
}

function icuSelectTemplate(
  pivot: string,
  order: readonly string[],
  flat: Map<string, string>,
): string {
  const parts: string[] = [];
  for (const key of order) parts.push(`${key} {${flat.get(key) ?? ''}}`);
  return `{${pivot}, select, ${parts.join(' ')}}`;
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
  if (!span) return '';
  return state.sourceSlice(span.start, span.end);
}

/**
 * Trim leading / trailing purely-whitespace text runs. Whitespace
 * between structural runs stays. Mirrors `text.trim()` in the legacy
 * flat extractor.
 */
function trimEdgeWhitespace(runs: Run[]): Run[] {
  if (runs.length === 0) return runs;
  const trimmed = [...runs];

  const first = trimmed[0];
  if (first && 'text' in first) {
    const text = (first as TextRun).text.replace(/^\s+/, '');
    if (text === '') trimmed.shift();
    else (first as TextRun).text = text;
  }

  const last = trimmed[trimmed.length - 1];
  if (last && 'text' in last) {
    const text = (last as TextRun).text.replace(/\s+$/, '');
    if (text === '') trimmed.pop();
    else (last as TextRun).text = text;
  }

  return trimmed;
}

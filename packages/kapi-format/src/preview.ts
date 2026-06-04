/**
 * Reference implementation of the Level-1 preview renderer.
 *
 * Walks a Block's runs and emits the `<kat-block>` HTML shape
 * neokapi's existing HTML and Markdown preview builders produce
 * for every format.
 *
 * The purpose of having this in @neokapi/kapi-format is:
 *   1. Validate that Blocks carry enough info to render a preview
 *      without reading the original source.
 *   2. Give @neokapi/kapi-react's test suite a way to assert "the
 *      extracted Block renders to exactly this HTML" without
 *      depending on neokapi's Go implementation.
 *   3. Give the Go-side `core/formats/jsx.PreviewBuilder`
 *      implementation a reference to port against, keeping the TS
 *      and Go sides in lockstep via shared fixtures.
 */

import type { Block, Run } from "./block.ts";
import type { VocabularyEntry } from "./vocabulary.ts";
import { JSX_VOCABULARY, expandTemplate } from "./vocabulary.ts";

export type VocabularyLookup = (type: string) => VocabularyEntry | undefined;

export function createVocabulary(entries: VocabularyEntry[]): VocabularyLookup {
  const map = new Map(entries.map((e) => [e.key, e]));
  return (type) => map.get(type);
}

// ─── Run rendering ────────────────────────────────────────────────

/**
 * Render a run sequence to HTML by walking the array in order and
 * dispatching each run to its vocabulary entry. Recurses into
 * plural / select forms, which hold their own Run[] sub-sequences.
 */
export function renderRuns(runs: Run[], vocab: VocabularyLookup): string {
  let out = "";
  for (const run of runs) {
    if ("text" in run) {
      out += escapeHtml(run.text);
    } else if ("ph" in run) {
      out += renderEntry(vocab, run.ph.type, "placeholder", {
        id: run.ph.id,
        subType: run.ph.subType ?? "",
        data: run.ph.data,
        equiv: run.ph.equiv,
      });
    } else if ("pcOpen" in run) {
      out += renderEntry(vocab, run.pcOpen.type, "open", {
        id: run.pcOpen.id,
        subType: run.pcOpen.subType ?? "",
        data: run.pcOpen.data,
        equiv: run.pcOpen.equiv,
      });
    } else if ("pcClose" in run) {
      out += renderEntry(vocab, run.pcClose.type, "close", {
        id: run.pcClose.id,
        subType: run.pcClose.subType ?? "",
        data: run.pcClose.data,
        equiv: run.pcClose.equiv ?? "",
      });
    } else if ("sub" in run) {
      out += `<span class="neokapi-sub" data-ref="${escapeHtml(run.sub.ref)}">${escapeHtml(run.sub.equiv)}</span>`;
    } else if ("plural" in run) {
      out += renderPluralRun(run.plural, vocab);
    } else if ("select" in run) {
      out += renderSelectRun(run.select, vocab);
    }
  }
  return out;
}

// CLDR plural-form order, mirroring core/klf.pluralOrder, so the Go and
// TypeScript renderers emit forms in the same sequence regardless of the
// order they were authored in.
const PLURAL_ORDER = ["zero", "one", "two", "few", "many", "other"];

function orderedPluralForms(forms: Partial<Record<string, Run[]>>): string[] {
  const present = new Set(Object.keys(forms));
  const ordered = PLURAL_ORDER.filter((f) => present.has(f));
  // Any non-standard keys follow the canonical ones, sorted, for stability.
  const extras = [...present].filter((f) => !PLURAL_ORDER.includes(f)).sort();
  return [...ordered, ...extras];
}

// Select-case order, mirroring core/klf.orderedSelectKeys: the keys sorted
// alphabetically, with `other` last.
function orderedSelectKeys(cases: Record<string, Run[]>): string[] {
  const keys = Object.keys(cases)
    .filter((k) => k !== "other")
    .sort();
  if ("other" in cases) keys.push("other");
  return keys;
}

function renderPluralRun(
  plural: { pivot: string; forms: Partial<Record<string, Run[]>> },
  vocab: VocabularyLookup,
): string {
  const pivot = escapeHtml(plural.pivot);
  const formEntries = orderedPluralForms(plural.forms).map(
    (form) => [form, plural.forms[form] as Run[]] as [string, Run[]],
  );
  const inner = formEntries
    .map(([form, formRuns]) => {
      const label = `plural:${form}`;
      const body = renderRuns(formRuns, vocab);
      return (
        `<div class="neokapi-plural-form" data-form="${escapeHtml(form)}">` +
        `<span class="neokapi-plural-form-label">${escapeHtml(label)}</span>${body}</div>`
      );
    })
    .join("");
  return `<span class="neokapi-plural" data-pivot="${pivot}">${inner}</span>`;
}

function renderSelectRun(
  sel: { pivot: string; cases: Record<string, Run[]> },
  vocab: VocabularyLookup,
): string {
  const pivot = escapeHtml(sel.pivot);
  const inner = orderedSelectKeys(sel.cases)
    .map((value) => [value, sel.cases[value]] as [string, Run[]])
    .map(([value, caseRuns]) => {
      const label = `select:${value}`;
      const body = renderRuns(caseRuns, vocab);
      return (
        `<div class="neokapi-select-case" data-value="${escapeHtml(value)}">` +
        `<span class="neokapi-select-case-label">${escapeHtml(label)}</span>${body}</div>`
      );
    })
    .join("");
  return `<span class="neokapi-select" data-pivot="${pivot}">${inner}</span>`;
}

function renderEntry(
  vocab: VocabularyLookup,
  type: string,
  kind: "open" | "close" | "placeholder",
  context: { id: string; subType: string; data: string; equiv: string },
): string {
  const entry = vocab(type);
  if (!entry) {
    // Fallback: show the raw data. Keeps us resilient if a new
    // vocabulary key appears before the registry is updated.
    return `<span class="neokapi-unknown">${escapeHtml(context.data)}</span>`;
  }
  const template =
    kind === "open"
      ? entry.html.open
      : kind === "close"
        ? entry.html.close
        : entry.html.placeholder;
  return expandTemplate(template, {
    id: context.id,
    subType: context.subType,
    data: escapeHtml(context.data),
    equiv: escapeHtml(context.equiv),
  });
}

// ─── Block rendering ──────────────────────────────────────────────

/**
 * Render a whole Block wrapped in a `<kat-block>` marker — the
 * same interactive wrapper neokapi's existing preview builders
 * emit for every format. Block.source is a flat Run[]; plurals
 * and select constructs inside that sequence recurse naturally.
 */
export function renderBlockHtml(
  block: Block,
  vocab: VocabularyLookup = createVocabulary(JSX_VOCABULARY),
): string {
  const inner = renderRuns(block.source, vocab);
  return `<kat-block id="${block.id}" data-type="${block.type}">${inner}</kat-block>`;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// ─── Validation ───────────────────────────────────────────────────

/**
 * Assert that a target run sequence preserves every required
 * source placeholder. A target may add content or restructure
 * text, but it cannot drop a required placeholder. Optional
 * placeholders (conditional JSX nodes) may be dropped.
 *
 * Returns a list of errors; empty array means valid.
 */
export function validateTargetAgainstSource(
  sourceBlock: Block,
  targetRuns: Run[],
): ValidationError[] {
  const errors: ValidationError[] = [];
  const targetNames = collectRunEquivs(targetRuns);

  for (const placeholder of sourceBlock.placeholders) {
    if (placeholder.optional) continue;
    if (!targetNames.has(placeholder.name)) {
      errors.push({
        blockId: sourceBlock.id,
        kind: "missing-placeholder",
        placeholder: placeholder.name,
        message: `target is missing required placeholder "${placeholder.name}"`,
      });
    }
  }

  return errors;
}

/**
 * Walk a run sequence (including nested plural / select forms)
 * and collect every reference that counts toward placeholder
 * preservation: `equiv` of ph / pcOpen / sub runs, plus the
 * `pivot` of any plural / select construct encountered.
 */
function collectRunEquivs(runs: Run[]): Set<string> {
  const names = new Set<string>();
  const visit = (rs: Run[]) => {
    for (const run of rs) {
      if ("ph" in run) names.add(run.ph.equiv);
      else if ("pcOpen" in run) names.add(run.pcOpen.equiv);
      else if ("sub" in run) names.add(run.sub.equiv);
      else if ("plural" in run) {
        names.add(run.plural.pivot);
        for (const formRuns of Object.values(run.plural.forms)) {
          if (formRuns) visit(formRuns);
        }
      } else if ("select" in run) {
        names.add(run.select.pivot);
        for (const caseRuns of Object.values(run.select.cases)) {
          visit(caseRuns);
        }
      }
    }
  };
  visit(runs);
  return names;
}

export interface ValidationError {
  blockId: string;
  kind: "missing-placeholder" | "extra-placeholder" | "malformed-runs";
  placeholder?: string;
  message: string;
}

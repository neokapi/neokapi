// The terms of a container unit, read from the term check's overlays. The
// server runs the check (term-check, the gate every surface uses) and returns
// its spans run-anchored; this file only groups them. It matches no text, so
// what a client marks is what the check decided.

import type { AnchorLike } from "@neokapi/kapi-format";

import type { ContainerUnit } from "./types.ts";

/** One term a unit's source uses, as the term check holds the translation to it. */
export interface UnitTerm {
  /** Pairs the term in the source with its renderings in the translation. */
  id: string;
  /** The term as the rule names it. */
  term: string;
  /** The rendering the rule asks the translation to use. */
  replacement?: string;
  /** The concept in the terms store, when the rule comes from one. */
  conceptId?: string;
  /** The check's verdict: the translation holds a rendering. */
  satisfied: boolean;
  /** A rule that reports and never fails. */
  advisory: boolean;
  /** The translation keeps the term as written. */
  doNotTranslate: boolean;
  /** Where the term sits in the source runs. */
  source: AnchorLike[];
  /** Where the translation renders it; empty when it does not. */
  target: AnchorLike[];
}

/** The term check's spans carry this id prefix. */
const TERM_CHECK = "term-check:";

/** The terms a unit's source uses, in the order the check reports them. */
export function unitTerms(unit: ContainerUnit): UnitTerm[] {
  const byId = new Map<string, UnitTerm>();
  for (const overlay of unit.overlays ?? []) {
    if (overlay.type !== "term") continue;
    const onSource = overlay.side === "source";
    for (const span of overlay.spans ?? []) {
      const id = span.id ?? "";
      if (!id.startsWith(TERM_CHECK)) continue;
      const props = span.props ?? {};
      let term = byId.get(id);
      if (!term) {
        term = {
          id,
          term: props.term ?? span.text ?? "",
          replacement: props.replacement || undefined,
          conceptId: props.concept_id || undefined,
          satisfied: props.satisfied === "true",
          advisory: props.advisory === "true",
          doNotTranslate: props.do_not_translate === "true",
          source: [],
          target: [],
        };
        byId.set(id, term);
      }
      (onSource ? term.source : term.target).push(span.range);
    }
  }
  return [...byId.values()].filter((t) => t.source.length > 0);
}

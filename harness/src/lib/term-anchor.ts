/**
 * The block the governance walk's closing beat films, and what makes it that
 * block.
 *
 * `VisualEditorLayout` docks the term sidebar for a block the terms store
 * answers for and for no other, and the seeded file's headings carry
 * content-memory matches instead. A walk that stops at the first block with
 * anything beside it therefore lands on the memory matches and films them under
 * narration about terms (#2605).
 *
 * So the seed proves a matched block is there before a take is worth recording,
 * and names it for the walk. The predicates live here rather than in the seed
 * script so they can be tested without a stack: the seed lists the file's
 * blocks, asks the server for each one's term matches, and reads the answer
 * through these.
 */

/** One block as the editor's block list returns it, to the fields this reads. */
export interface SourceBlock {
  id: string;
  translatable?: boolean;
  source?: string;
}

/** One match as `GET /:ws/:project/blocks/:ref/:bid/term-matches` returns it. */
export interface TermMatch {
  source_term?: string;
  target_terms?: string[] | null;
  status?: string;
}

/** The block the sidebar has a decision to show, and the decision. */
export interface TermAnchor {
  block: SourceBlock;
  /** The source term the sidebar names. */
  term: string;
  /** The agreed wording in the recorded language. */
  targets: string[];
}

/** The blocks a term lookup is worth asking about: translatable, with text. */
export function lookupCandidates(blocks: readonly SourceBlock[]): SourceBlock[] {
  return blocks.filter((b) => b && b.translatable !== false && (b.source ?? "").trim() !== "");
}

/**
 * The matches the sidebar shows a decision for: a source term with at least one
 * term in the recorded language. A match without one draws "No target term
 * defined", and the narration says the agreed word is on screen.
 */
export function decidedMatches(matches: readonly TermMatch[]): TermMatch[] {
  return matches.filter(
    (m) =>
      (m?.source_term ?? "").trim() !== "" &&
      (m?.target_terms ?? []).some((t) => (t ?? "").trim() !== ""),
  );
}

/** The first candidate block the terms store answers for with a decided term. */
export function pickTermAnchor(
  candidates: readonly SourceBlock[],
  matchesOf: (blockId: string) => readonly TermMatch[],
): TermAnchor | null {
  for (const block of candidates) {
    const first = decidedMatches(matchesOf(block.id))[0];
    if (!first) continue;
    return {
      block,
      term: (first.source_term ?? "").trim(),
      targets: (first.target_terms ?? []).filter((t) => (t ?? "").trim() !== ""),
    };
  }
  return null;
}

/** The blocks whose source carries `phrase`, ignoring case. */
export function blocksCarrying(blocks: readonly SourceBlock[], phrase: string): SourceBlock[] {
  const needle = phrase.trim().toLowerCase();
  if (!needle) return [];
  return blocks.filter((b) => (b?.source ?? "").toLowerCase().includes(needle));
}

/** One line for the seed log, naming the block and what it is anchored on. */
export function describeTermAnchor(anchor: TermAnchor, locale: string): string {
  return `block ${anchor.block.id} carries "${anchor.term}", agreed as ${anchor.targets.join(", ")} in ${locale}`;
}

/**
 * Why no block can carry the beat, in the terms of whichever half is missing:
 * the file no longer carries the words, the terms store holds nothing for them,
 * or the concept has no wording in the recorded language.
 */
export function describeMissingAnchor(
  fileName: string,
  phrase: string,
  locale: string,
  candidates: readonly SourceBlock[],
  matchesOf: (blockId: string) => readonly TermMatch[],
): string {
  const carrying = blocksCarrying(candidates, phrase);
  const answered = candidates.filter((b) => matchesOf(b.id).length > 0);
  if (carrying.length === 0) {
    return `${fileName} has no block reading "${phrase}" among its ${candidates.length} translatable block(s), so the workspace terms have nothing to mark in it`;
  }
  if (answered.length === 0) {
    return `${fileName} carries "${phrase}" on block ${carrying[0]!.id}, and the terms store answers no match for any of its ${candidates.length} block(s): check that the workspace concepts were created`;
  }
  return `${answered.length} of ${candidates.length} block(s) in ${fileName} match a term, and none of those terms has a ${locale} wording, so the sidebar would read "No target term defined"`;
}

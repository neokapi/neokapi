// The context deck as this person sees it right now: the server's cards in
// the server's order, minus the ones decided on this device and with the
// ones put off moved to the end. Which cards exist, in what order, and which
// actions each offers is the server's; the app applies what the person did.
import type { ContextCard, ContextSessionSince } from "@neokapi/bowrain-client";

/** A card's identity at one revision: a new revision is a new card. */
export function contextCardId(card: Pick<ContextCard, "item" | "revision">): string {
  return `${card.item.kind}:${card.item.profile ?? ""}:${card.item.id}@${card.revision}`;
}

/** Cards from consecutive pages, each once, the first occurrence kept. */
export function mergeContextPages(pages: readonly (readonly ContextCard[])[]): ContextCard[] {
  const seen = new Set<string>();
  const out: ContextCard[] = [];
  for (const page of pages) {
    for (const c of page) {
      const id = contextCardId(c);
      if (seen.has(id)) continue;
      seen.add(id);
      out.push(c);
    }
  }
  return out;
}

/** The cards to swipe, top first. */
export function contextDeckView<T extends ContextCard>(
  cards: readonly T[],
  decided: ReadonlySet<string>,
  later: readonly string[],
): T[] {
  const open = cards.filter((c) => !decided.has(contextCardId(c)));
  const rank = new Map(later.map((k, i) => [k, i]));
  const now = open.filter((c) => !rank.has(contextCardId(c)));
  const putOff = open
    .filter((c) => rank.has(contextCardId(c)))
    .sort((a, b) => (rank.get(contextCardId(a)) ?? 0) - (rank.get(contextCardId(b)) ?? 0));
  return [...now, ...putOff];
}

/**
 * The i18n prefix of a card's action labels: a proposed translation of a term
 * and a candidate term each say what keep and reject do to them, and every
 * other card uses the plain wording.
 */
export function contextCopyPrefix(
  card: Pick<ContextCard, "kind" | "proposed"> | undefined,
): "context.translation" | "context.candidate" | "context" {
  if (card?.kind === "term_translation") return "context.translation";
  if (card?.proposed) return "context.candidate";
  return "context";
}

/** The refusal code a failed decision carries, or "failed" when it carries none. */
export function refusalCode(err: unknown): string {
  const body = (err as { body?: { code?: unknown } } | undefined)?.body;
  return typeof body?.code === "string" && body.code !== "" ? body.code : "failed";
}

/**
 * The widest reach among a session's cards, which each card's reach bar is
 * drawn against. Zero when no card carries a reach.
 */
export function widestReach(cards: readonly Pick<ContextCard, "reach">[]): number {
  return cards.reduce((max, c) => Math.max(max, c.reach?.blocks ?? 0), 0);
}

/** The share of the bar a reach fills, 0 to 1, against the widest in the session. */
export function reachShare(blocks: number | undefined, widest: number): number {
  if (!blocks || widest <= 0) return 0;
  return Math.min(1, blocks / widest);
}

/** Whether the "since the last session" header has anything to say. */
export function sinceHasNews(since: ContextSessionSince | undefined): since is ContextSessionSince {
  return !!since && (since.added_total > 0 || since.grew_total > 0 || since.decided_by_others > 0);
}

/**
 * Whether a session is through its cards: each one decided, or put off once.
 * A session with no cards is through at once.
 */
export function sessionThrough(
  cards: readonly ContextCard[],
  decided: ReadonlySet<string>,
  later: readonly string[],
): boolean {
  const putOff = new Set(later);
  return cards.every((c) => decided.has(contextCardId(c)) || putOff.has(contextCardId(c)));
}

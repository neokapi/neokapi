// The context deck as this person sees it right now: the server's cards in
// the server's order, minus the ones decided on this device and with the
// ones put off moved to the end. Which cards exist, in what order, and which
// actions each offers is the server's; the app applies what the person did.
import type { ContextCard } from "@neokapi/bowrain-client";

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
export function contextDeckView(
  cards: readonly ContextCard[],
  decided: ReadonlySet<string>,
  later: readonly string[],
): ContextCard[] {
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

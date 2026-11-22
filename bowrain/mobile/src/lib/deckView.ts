// The deck as this person sees it right now: the server's cards, in the
// server's order, minus the ones they decided and with the ones they put off
// moved to the end. Which cards exist and in what order is the server's; the
// app only applies what the person did on this device.
import type { DeckCard } from "@neokapi/bowrain-client";

/** A card's identity at one revision: a new revision is a new card. */
export function cardId(card: Pick<DeckCard, "key" | "targetRevision">): string {
  return `${card.key}@${card.targetRevision}`;
}

/** Cards from consecutive pages, each once, the first occurrence kept. */
export function mergePages(pages: readonly (readonly DeckCard[])[]): DeckCard[] {
  const seen = new Set<string>();
  const out: DeckCard[] = [];
  for (const page of pages) {
    for (const c of page) {
      if (seen.has(c.key)) continue;
      seen.add(c.key);
      out.push(c);
    }
  }
  return out;
}

export interface DeckView {
  /** The cards to swipe, top first. */
  swipe: DeckCard[];
  /** The cards the server routes to a desk, listed and not swiped. */
  desk: DeckCard[];
}

export function deckView(
  cards: readonly DeckCard[],
  decided: ReadonlySet<string>,
  later: readonly string[],
): DeckView {
  const open = cards.filter((c) => !decided.has(cardId(c)));
  const desk = open.filter((c) => c.group === "desk");
  const swipe = open.filter((c) => c.group !== "desk");
  const rank = new Map(later.map((k, i) => [k, i]));
  const now = swipe.filter((c) => !rank.has(c.key));
  const putOff = swipe
    .filter((c) => rank.has(c.key))
    .sort((a, b) => (rank.get(a.key) ?? 0) - (rank.get(b.key) ?? 0));
  return { swipe: [...now, ...putOff], desk };
}

// A language's review deck taken one collection at a time. The server lists
// the collections, counts them and orders them
// (GET /:ws/:id/review-deck/:ref/collections); the app carries the chosen
// collection in the deck's route and offers the next one when a deck runs out.
import type { DeckCollection } from "@neokapi/bowrain-client";

/**
 * The route parameter for the blocks in no collection. The server names them
 * with an empty collection id, which a route parameter does not keep, and a
 * collection id is never a single dash.
 */
const NO_COLLECTION = "-";

/** The query key of a language's collections. */
export function collectionsKey(ws: string, project: string, locale: string) {
  return ["deck-collections", ws, project, locale] as const;
}

/** The route parameter that opens the deck on one collection. */
export function collectionParam(id: string): string {
  return id === "" ? NO_COLLECTION : id;
}

/**
 * The collection a deck route names: undefined for the whole language, an
 * empty string for the blocks in no collection.
 */
export function collectionFromParam(param: string | string[] | undefined): string | undefined {
  const value = Array.isArray(param) ? param[0] : param;
  if (value === undefined || value === "") return undefined;
  return value === NO_COLLECTION ? "" : value;
}

/**
 * The collection to offer after the one just finished: the first other one,
 * in the server's order, with anything waiting. Null when none is left.
 */
export function nextCollection(
  collections: readonly DeckCollection[] | undefined,
  current: string | undefined,
): DeckCollection | null {
  if (current === undefined || !collections) return null;
  return collections.find((c) => c.collection_id !== current && c.waiting > 0) ?? null;
}

/** How much of a collection is decided, from 0 to 1. */
export function decidedShare(c: Pick<DeckCollection, "decided" | "translations">): number {
  return c.translations > 0 ? Math.min(1, Math.max(0, c.decided / c.translations)) : 0;
}

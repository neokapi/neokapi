// The deck by page: what the app needs to open a page's containers.
import type { DeckCard } from "@neokapi/bowrain-client";

/**
 * The card a page review opens a container with. The container route reads
 * only the block and the language; decisions go out on each unit's own card
 * (cardOfUnit), which carries its revisions.
 */
export function pageCard(blockId: string, locale: string, collectionId: string): DeckCard {
  return {
    key: `${blockId}:${locale}`,
    blockId,
    item: "",
    locale,
    collectionId,
    source: [],
    target: [],
    targetRevision: "",
    group: "unsorted",
    notes: [],
    findingsCount: 0,
    failingFindingsCount: 0,
    ownWork: false,
  };
}

/** Whether the deck opens on pages: a server with the route and pages to show. */
export function opensOnPages(pages: { pages: unknown[] } | null | undefined): boolean {
  return !!pages && pages.pages.length > 0;
}

import { PREVIEW_KIND_STORYBOOK, type CollectionPreview } from "../../types/api";

/**
 * Resolving a collection's declared preview host into something this client can
 * actually open.
 *
 * The hosts differ in how a view is FOUND, not in how it renders. A Storybook
 * publishes an index that maps components to stories, so an item resolves to a
 * story by the components its blocks name; a running site would be addressed by
 * route, which this client cannot do. So a kind it does not recognise resolves
 * to nothing: an empty iframe with no explanation is worse for a reviewer than
 * an offer that was never made.
 */

/** The Storybook root for this collection, or undefined if it has no readable one. */
export function storybookHost(preview: CollectionPreview | undefined): string | undefined {
  if (!preview || preview.kind !== PREVIEW_KIND_STORYBOOK) return undefined;
  const url = preview.url?.trim();
  return url ? url : undefined;
}

/** Whether this client can offer in-context reading for a collection. */
export function canReadInContext(preview: CollectionPreview | undefined): boolean {
  return storybookHost(preview) !== undefined;
}

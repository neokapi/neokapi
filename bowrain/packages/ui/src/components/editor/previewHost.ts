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

/**
 * The embed origin this deployment frames previews through.
 *
 * A build-time constant rather than something the server answers, because it is
 * the twin of the application's own content policy: the policy names this origin
 * in frame-src (bowrain-infra, modules/spa-site's csp_frame_src) and the two
 * have to agree, or the frame is refused and there is nothing in the interface
 * to read that from. Set VITE_EMBED_ORIGIN where the app is built.
 *
 * Empty in development and in the desktop app, where no such origin exists — so
 * in-context reading is not offered there rather than offered and broken.
 */
export function embedOrigin(): string {
  const configured = (import.meta.env?.VITE_EMBED_ORIGIN ?? "") as string;
  return configured.trim().replace(/\/+$/, "");
}

/**
 * Whether this client can offer in-context reading for a collection: it needs
 * both a host it knows how to resolve a view within and an origin it is allowed
 * to frame that view through.
 */
export function canReadInContext(preview: CollectionPreview | undefined): boolean {
  return storybookHost(preview) !== undefined && embedOrigin() !== "";
}

import type { BlockInfo } from "../../types/api";
import { getTargetText } from "./blockStatus";

/**
 * Finding the story that renders an item's strings.
 *
 * A KBF bundle records, per block, the component file its string was extracted
 * from. A published Storybook records, per story, the file the story was
 * written in. Neither points at the other, and neither needs to: the two files
 * sit side by side by convention — `CollectionRail.tsx` beside
 * `CollectionRail.stories.tsx` — so the basename joins them.
 *
 * The alternative was to have the extractor resolve a story id at extraction
 * time and write it into the bundle. That couples the extractor to Storybook's
 * id derivation (which depends on the *consuming* Storybook's config, not on
 * the component), bakes a stale id into every catalog, and cannot answer for a
 * component whose stories arrive later. Matching at read time asks the
 * Storybook that is actually being displayed.
 */

/** One story, as `<storybook>/index.json` describes it. */
export interface StoryEntry {
  id: string;
  title: string;
  name: string;
  importPath: string;
  type?: string;
}

/** The `index.json` a built or running Storybook serves. */
export interface StoryIndex {
  entries: Record<string, StoryEntry>;
}

/** The file a path names, without directories or the `.stories.tsx` suffix. */
function componentOf(path: string): string {
  const file = path.split("/").pop() ?? "";
  return file.replace(/\.stories\.(tsx|ts|jsx|js)$/, "").replace(/\.(tsx|ts|jsx|js)$/, "");
}

/**
 * The components an item's blocks were extracted from, in the order they first
 * appear — the order the document reads in, so the first match is the story a
 * reader most likely means.
 */
export function componentsOf(blocks: BlockInfo[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const block of blocks) {
    const file = block.properties?.file;
    if (!file) continue;
    const name = componentOf(file);
    if (!name || seen.has(name)) continue;
    seen.add(name);
    out.push(name);
  }
  return out;
}

/**
 * The stories that render any of an item's components, in index order. Docs
 * entries are skipped: a docs page is a page *about* the component, and
 * embedding one shows a manual rather than the thing being reviewed.
 */
export function storiesForComponents(index: StoryIndex, components: string[]): StoryEntry[] {
  if (components.length === 0) return [];
  const wanted = new Set(components);
  return Object.values(index.entries ?? {}).filter(
    (entry) => entry.type !== "docs" && wanted.has(componentOf(entry.importPath)),
  );
}

/**
 * The component basenames this index renders a non-docs story for.
 *
 * The same join `storiesForComponents` makes, computed once for a whole
 * collection instead of once per opened item. That is what lets a list say
 * which of its rows can be read in context BEFORE a reviewer clicks one — the
 * offer stops being made and then withdrawn.
 */
export function storyComponents(index: StoryIndex): Set<string> {
  const out = new Set<string>();
  for (const entry of Object.values(index.entries ?? {})) {
    if (entry.type === "docs") continue;
    const name = componentOf(entry.importPath);
    if (name) out.add(name);
  }
  return out;
}

/**
 * Whether a story renders the component an item was extracted from.
 *
 * Asked of the item's SOURCE path, never of its name. `source_path` is the same
 * fact `componentsOf` reads off the blocks, hoisted onto the item at ingest, so
 * both sides of the join run the one `componentOf` over the one kind of path —
 * and a list can ask it of 300 rows without fetching 300 documents. Deriving a
 * component from the catalog's own name instead would be a second spelling of
 * this rule: one that has to know what a catalog is called, and that drifts the
 * moment the extractor names one differently.
 *
 * An item with no source path is not a component catalog and has no story — the
 * `file` property this ultimately comes from is written by one reader, the JSX
 * one. Answering "yes" for a Markdown page that happens to share a basename with
 * a component would offer a reading `componentsOf` then declines to find.
 */
export function hasStoryFor(components: ReadonlySet<string>, sourcePath?: string): boolean {
  if (components.size === 0 || !sourcePath) return false;
  const component = componentOf(sourcePath);
  return component !== "" && components.has(component);
}

/**
 * The URL that renders one story on its own, with no Storybook chrome around
 * it. `viewMode=story` keeps the docs page out of it even for a story whose
 * component has autodocs.
 */
export function storyURL(storybookURL: string, storyId: string): string {
  const base = storybookURL.replace(/\/+$/, "");
  return `${base}/iframe.html?id=${encodeURIComponent(storyId)}&viewMode=story`;
}

/**
 * That story, addressed through the embed origin.
 *
 * The application never frames a customer's host directly. Its content policy
 * names one origin — the embed shim — because the hosts a recipe may declare
 * cannot be enumerated in advance, and widening frame-src to "any https" would
 * let an injection here frame anything at all, beside this session and this
 * API. The shim frames the story and relays the i18n runtime's two messages.
 *
 * Without an embed origin configured there is no in-context reading: returning
 * the story URL directly would be blocked by the policy and look like a blank
 * frame rather than like a deployment that has not set this.
 */
export function embeddedStoryURL(
  embedOrigin: string,
  storybookURL: string,
  storyId: string,
): string | undefined {
  const origin = embedOrigin.replace(/\/+$/, "");
  if (!origin) return undefined;
  return `${origin}/?src=${encodeURIComponent(storyURL(storybookURL, storyId))}`;
}

/**
 * The story index, read through this project's own API rather than from the
 * customer's host.
 *
 * Not a detour: the browser cannot make that fetch. connect-src names 'self'
 * and no list of customer hosts can be added to it, and the host would have to
 * return Access-Control-Allow-Origin, which a static bucket or an internal CDN
 * has no reason to send. The server reads it instead — see
 * bowrain/server/handlers_preview_index.go.
 */
export function storyIndexURL(
  workspaceSlug: string,
  projectId: string,
  stream: string,
  collectionId: string,
): string {
  // Workspace-scoped, like every other collection route (Bowrain AD-011:
  // /:ws/:id/collections/:ref). There is no flat /projects/:id form for these,
  // and asking for one answers 404 — which reads in the interface as the
  // Storybook being unreadable rather than as an address that does not exist.
  return (
    `/api/v1/${encodeURIComponent(workspaceSlug)}/${encodeURIComponent(projectId)}` +
    `/collections/${encodeURIComponent(stream)}/${encodeURIComponent(collectionId)}/preview/index`
  );
}

/**
 * The dictionary a story needs to render an item in one locale: message key →
 * the target text this surface currently holds, pending edits included.
 *
 * The key is the block's `hash` property — the bundle's block hash, which is
 * what the i18n runtime looks a string up by at render time (both are
 * `hashKey(text, descriptor)`). A block without one cannot be addressed in a
 * running component and is left out rather than guessed at.
 */
export function translationsFor(blocks: BlockInfo[], locale: string): Record<string, string> {
  const dict: Record<string, string> = {};
  for (const block of blocks) {
    const key = block.properties?.hash;
    if (!key) continue;
    const text = getTargetText(block, locale);
    if (text) dict[key] = text;
  }
  return dict;
}

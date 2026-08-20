import { useMemo } from "react";
import type { CollectionPreview } from "../types/api";
import { useOptionalWorkspace } from "../context/WorkspaceContext";
import { canReadInContext } from "../components/editor/previewHost";
import { useStoryIndex } from "../components/editor/StoryPreview";
import { hasStoryFor, storyComponents } from "../components/editor/storyIndex";

/**
 * Which of a collection's items can be read inside the component that ships
 * them.
 *
 * A collection declaring a preview host does not mean every item in it has a
 * view: a Storybook renders the components someone wrote a story for, and the
 * rest of the collection has none. Offered on every row, "In context" was a
 * promise kept for some and broken for the others — a reviewer clicked, waited,
 * and read "No story renders this item's components". The disappointment is
 * avoidable, because the index that decides it is one fetch for the whole
 * collection and the surface was already making it, one item at a time, after
 * the choice had been offered.
 *
 * So it is made once, up front, and the answer is a predicate a list can ask of
 * every row it draws. `enabled` says whether the question is worth asking at
 * all — a collection with no resolvable host has no in-context reading for
 * anything, and a list should mark nothing rather than mark everything absent.
 */
export interface InContextItems {
  /** The collection has a host this client can resolve views within. */
  enabled: boolean;
  /** The index has been read; before this, `has` answers false for everything. */
  ready: boolean;
  /** Whether a story renders the component this item was extracted from. */
  has: (sourcePath?: string) => boolean;
}

export function useInContextItems(
  projectId: string,
  collectionId: string | undefined,
  preview: CollectionPreview | undefined,
  stream = "main",
): InContextItems {
  // Optional: the desktop app and the stories mount these views with no
  // workspace around them, and a list that simply marks nothing there is right.
  const workspace = useOptionalWorkspace();
  const enabled = Boolean(collectionId) && canReadInContext(preview);
  // The same query key the preview itself uses, so a reviewer who opens an item
  // reads the index this list already holds rather than fetching it again.
  const { data } = useStoryIndex(
    enabled ? (workspace?.activeWorkspace?.slug ?? "") : "",
    projectId,
    stream,
    enabled ? (collectionId ?? "") : "",
  );

  const components = useMemo(() => (data ? storyComponents(data) : undefined), [data]);

  return useMemo(
    () => ({
      enabled,
      ready: components !== undefined,
      has: (sourcePath?: string) => (components ? hasStoryFor(components, sourcePath) : false),
    }),
    [enabled, components],
  );
}

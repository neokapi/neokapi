import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import type { BlockInfo } from "../../types/api";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useDocumentBlocks } from "./DocumentPreview";
import { embedOrigin } from "./previewHost";
import {
  componentsOf,
  storiesForComponents,
  embeddedStoryURL,
  storyIndexURL,
  translationsFor,
  type StoryEntry,
  type StoryIndex,
} from "./storyIndex";

/** The message the story posts once it can accept a dictionary. */
const READY_MESSAGE = "neokapi-i18n-ready";
/** The message carrying a dictionary into the story. */
const TRANSLATIONS_MESSAGE = "neokapi-i18n-translations";

export interface StoryPreviewProps {
  /** The project whose API serves the story index. */
  projectId: string;
  /** The stream the collection belongs to. Defaults to main. */
  stream?: string;
  /** The collection whose declared preview host publishes the stories. */
  collectionId: string;
  /** Base URL of that host, as the collection declares it. */
  storybookURL: string;
  /** The item's blocks, carrying their message keys and current targets. */
  blocks: BlockInfo[];
  /** The locale to render, or "source" to read the component as authored. */
  locale: string;
  /** Read the source instead of a target. */
  source: boolean;
}

/**
 * StoryPreview renders an item's strings inside the component that ships them.
 *
 * The document preview answers "what does this file say"; this answers "what
 * will a reader see" — the heading in its heading, the button label in the
 * button, at the width the component lays out to. That is where a translation
 * that is correct but half a line too long stops being correct.
 *
 * The story is a published Storybook's own iframe, and the translations are
 * this surface's: a reviewer's pending target has never been built into any
 * catalog, so it is posted in (see `@neokapi/i18n-react/storybook`). What the
 * reviewer is looking at is therefore the component as it will ship *if they
 * approve what they are holding*, not as it shipped last time.
 */
export function StoryPreview({
  projectId,
  stream = "main",
  collectionId,
  storybookURL,
  blocks,
  locale,
  source,
}: StoryPreviewProps) {
  const frameRef = useRef<HTMLIFrameElement>(null);
  const [picked, setPicked] = useState<string | undefined>();

  const { activeWorkspace } = useWorkspace();
  const {
    data: index,
    isPending,
    error,
  } = useStoryIndex(activeWorkspace?.slug ?? "", projectId, stream, collectionId);

  const stories = useMemo(() => {
    if (!index) return [];
    return storiesForComponents(index, componentsOf(blocks));
  }, [index, blocks]);

  const story = stories.find((s) => s.id === picked) ?? stories[0];

  // The dictionary this surface holds for the locale being read. Source is not
  // a dictionary: it is the absence of one, which is what the component renders
  // from its own literals.
  const translations = useMemo(
    () => (source ? {} : translationsFor(blocks, locale)),
    [blocks, locale, source],
  );

  const post = useCallback(() => {
    // Addressed to the embed origin, which relays it to the story. Naming the
    // origin rather than "*" keeps the dictionary — a reviewer's unapproved
    // wording — from being readable by whatever else might be framed.
    const origin = embedOrigin();
    if (!origin) return;
    frameRef.current?.contentWindow?.postMessage(
      { type: TRANSLATIONS_MESSAGE, locale: source ? "en" : locale, translations },
      origin,
    );
  }, [locale, source, translations]);

  // The story announces when it can accept a dictionary — `load` fires before
  // the story has mounted, and a dictionary posted then is simply lost.
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if ((event.data as { type?: string } | null)?.type === READY_MESSAGE) post();
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [post]);

  // A later edit re-posts without reloading the frame, so the component updates
  // where the reviewer is looking.
  useEffect(() => post(), [post]);

  if (isPending) {
    return <Empty>Looking for the component…</Empty>;
  }
  if (error) {
    return (
      <Empty>
        This project&rsquo;s Storybook could not be read at <Mono>{storybookURL}</Mono>.
      </Empty>
    );
  }
  if (!story) {
    return (
      <Empty>
        No story renders this item&rsquo;s components, so there is nothing to read it in. A story
        beside the component, named <Mono>Component.stories.tsx</Mono>, is what this looks for.
      </Empty>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-2" data-testid="story-preview">
      {stories.length > 1 && (
        <Select value={story.id} onValueChange={setPicked}>
          <SelectTrigger className="h-8 w-full max-w-sm text-xs" data-testid="story-picker">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {stories.map((entry) => (
              <SelectItem key={entry.id} value={entry.id} className="text-xs">
                {storyLabel(entry)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      <iframe
        ref={frameRef}
        // Keyed by story: switching stories starts a fresh frame rather than
        // showing the previous component until the new one arrives.
        key={story.id}
        // Framed through the embed origin, never directly: the application's
        // content policy names that one origin, because the hosts a recipe may
        // declare cannot be enumerated in advance. See embeddedStoryURL.
        src={embeddedStoryURL(embedOrigin(), storybookURL, story.id)}
        onLoad={post}
        className="min-h-0 w-full flex-1 rounded-lg border border-border bg-white"
        // A Storybook is a JavaScript application: it needs `allow-scripts` to
        // run and `allow-same-origin` to fetch its own modules — without the
        // second the frame is given an opaque origin and every module request
        // fails CORS against the host it was just loaded from. The pair grants
        // nothing over *this* page: the Storybook is published elsewhere, so it
        // stays in its own origin and can reach neither bowrain's DOM nor its
        // storage. (Point this at a Storybook served from bowrain's own origin
        // and that separation is gone — which is a reason to publish it
        // somewhere else, not to break the frame.)
        sandbox="allow-scripts allow-same-origin"
        title={`${storyLabel(story)} · in context`}
        data-testid="story-frame"
      />
    </div>
  );
}

/** "Credits Warning · At 80 Percent" — the component, then which of its states. */
function storyLabel(entry: StoryEntry): string {
  const component = entry.title.split("/").pop() ?? entry.title;
  return `${component} · ${entry.name}`;
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="flex h-full items-center justify-center px-6 text-center text-sm text-muted-foreground"
      data-testid="story-preview-empty"
    >
      <p className="max-w-sm">{children}</p>
    </div>
  );
}

function Mono({ children }: { children: React.ReactNode }) {
  return <span className="font-mono text-[0.9em]">{children}</span>;
}

/**
 * The published Storybook's story index. Cached for the session: it changes
 * when the Storybook is rebuilt, not while a reviewer works.
 */
export function useStoryIndex(
  workspaceSlug: string,
  projectId: string,
  stream: string,
  collectionId: string,
) {
  return useQuery({
    queryKey: ["story-index", workspaceSlug, projectId, stream, collectionId],
    enabled: Boolean(collectionId) && Boolean(workspaceSlug),
    queryFn: async (): Promise<StoryIndex> => {
      const response = await fetch(storyIndexURL(workspaceSlug, projectId, stream, collectionId), {
        credentials: "include",
      });
      if (!response.ok) throw new Error(`story index: ${response.status}`);
      return (await response.json()) as StoryIndex;
    },
    staleTime: 5 * 60_000,
    retry: false,
  });
}

/** How an item is being read: as the file it is, or in the component it ships in. */
export type ReadingMode = "document" | "context";

/**
 * StoryPreview for one item, fetching the blocks it needs. The same paged
 * query the document preview uses, so switching between the two readings is
 * a cache hit rather than a second fetch of the same document.
 */
export function ItemStoryPreview({
  projectId,
  stream,
  collectionId,
  itemName,
  storybookURL,
  locale,
  source,
}: {
  projectId: string;
  stream?: string;
  collectionId: string;
  itemName: string;
  storybookURL: string;
  locale: string;
  source: boolean;
}) {
  const { data: blocks } = useDocumentBlocks(projectId, itemName, locale);
  return (
    <StoryPreview
      projectId={projectId}
      stream={stream}
      collectionId={collectionId}
      storybookURL={storybookURL}
      blocks={blocks ?? []}
      locale={locale}
      source={source}
    />
  );
}

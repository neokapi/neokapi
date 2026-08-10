import { useQuery } from "@tanstack/react-query";
import { Loader2, FileWarning } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@neokapi/ui-primitives";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { ContentTree, ContentNode } from "@neokapi/ui-primitives/preview";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";

// collectMediaNodes walks the tree for media nodes that carry a resolvable URI
// (the image/audio/video readers emit the asset by URI). Each needs its bytes
// served to the frontend before the DocumentViewer can render it.
function collectMediaNodes(tree: ContentTree): ContentNode[] {
  const out: ContentNode[] = [];
  const walk = (n: ContentNode) => {
    if (n.kind === "media" && n.media?.uri) out.push(n);
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return out;
}

export interface FilePreviewProps {
  /** Tab ID of the open project (used for the inspect bindings). */
  tabID: string;
  /**
   * Absolute path of the file to preview. When null the sheet is closed.
   * Setting it (re)opens the sheet and triggers a fresh inspect.
   */
  filePath: string | null;
  /** Short label shown in the header (e.g. the relative path). */
  filename: string;
  /** Called when the user dismisses the sheet. */
  onClose: () => void;
  /**
   * When set, `filePath` is treated as an archive container and only this inner
   * entry is previewed (via InspectArchiveEntry) — the desktop equivalent of the
   * `archive.zip!entry` locator. Unset previews the whole file.
   */
  entryPath?: string | null;
  /**
   * Pre-loaded ContentTree for Storybook/tests, skipping the backend call.
   * When set, `filePath` only needs to be non-null to open the sheet.
   */
  tree?: ContentTree;
}

// FilePreview is the desktop's project-content preview surface. It reuses the
// docs PreviewKit's DocumentViewer (Preview · Blocks · Stats · Download, with a
// source↔target toggle and annotation highlighting) so a project file renders
// exactly the way the documentation explorers render it — but driven by the
// desktop's full native engine via the InspectFileAnnotated binding rather than
// the WASM runtime.
//
// It calls InspectFileAnnotated so the tree carries the project's real
// terminology, voice-vocabulary and QA overlays; the DocumentViewer's
// Annotations toggle highlights them on the rendered document. Committed targets
// from the project (translated/merged sibling files) ride along in the tree, so
// the source↔target toggle works whenever a translation exists.
export function FilePreview({
  tabID,
  filePath,
  filename,
  onClose,
  entryPath,
  tree: presetTree,
}: FilePreviewProps) {
  // Inspect the file (or one archive entry) and serve any media bytes in a single
  // query fn — the Wails bindings are the data source, react-query owns caching.
  const previewQuery = useQuery({
    queryKey: entryPath
      ? qk.inspectArchiveEntry(tabID, filePath ?? "", entryPath)
      : qk.inspectFile(tabID, filePath ?? "", true),
    enabled: !!filePath && !presetTree,
    queryFn: async () => {
      const json = entryPath
        ? await api.inspectArchiveEntry(tabID, filePath!, entryPath)
        : await api.inspectFileAnnotated(tabID, filePath!);
      if (!json) {
        return { tree: null as ContentTree | null, mediaUrls: {} as Record<string, string> };
      }
      const parsed = JSON.parse(json) as ContentTree;
      // Serve each media node's bytes so the viewer can render image/audio/video.
      const nodes = collectMediaNodes(parsed);
      let mediaUrls: Record<string, string> = {};
      if (nodes.length > 0) {
        const pairs = await Promise.all(
          nodes.map(async (n) => {
            const url = await api.mediaDataURL(n.media!.uri!);
            return url ? ([n.id, url] as const) : null;
          }),
        );
        mediaUrls = Object.fromEntries(pairs.filter((p): p is [string, string] => p !== null));
      }
      return { tree: parsed, mediaUrls };
    },
  });

  const tree = presetTree ?? previewQuery.data?.tree ?? null;
  const mediaUrls = previewQuery.data?.mediaUrls ?? {};
  const loading = !presetTree && !!filePath && previewQuery.isLoading;
  const error = previewQuery.error
    ? previewQuery.error instanceof Error
      ? previewQuery.error.message
      : String(previewQuery.error)
    : // A null inspect result (no Wails runtime) resolves successfully but empty.
      !presetTree && previewQuery.isSuccess && previewQuery.data.tree === null
      ? "Preview is unavailable in this environment."
      : null;

  return (
    <Sheet open={!!filePath} onOpenChange={(open) => !open && onClose()}>
      {/* Half the window on wide screens; progressively wider on smaller ones
          (full width on the smallest). Uses the data-[side] variant so it wins
          over the Sheet's default w-3/4 / sm:max-w-sm. */}
      <SheetContent
        side="right"
        className="gap-3 data-[side=right]:w-full data-[side=right]:sm:w-3/4 data-[side=right]:sm:max-w-none data-[side=right]:lg:w-1/2"
      >
        <SheetHeader className="pb-0">
          <SheetTitle className="font-mono text-sm" translate="no">
            {filename}
          </SheetTitle>
          <SheetDescription>
            Structure, vocabulary &amp; QA annotations, and source &harr; target.
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-auto px-4 pb-4">
          {loading && (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Inspecting {filename}…
            </div>
          )}
          {error && (
            <div className="flex items-center gap-2 py-8 text-sm text-destructive">
              <FileWarning className="size-4" />
              {error}
            </div>
          )}
          {!loading && !error && tree && (
            <DocumentViewer
              tree={tree}
              filename={filename}
              resolveMediaUrl={(node) => mediaUrls[node.id] ?? node.media?.uri}
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

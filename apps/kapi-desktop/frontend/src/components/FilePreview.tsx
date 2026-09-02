import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Loader2, FileWarning } from "lucide-react";
import {
  Badge,
  Button,
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@neokapi/ui-primitives";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { BlockAttrs, ContentTree, ContentNode } from "@neokapi/ui-primitives/preview";
import { t } from "@neokapi/i18n-react/runtime";
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

// unitKeyOf is the engine's own key for a block (convergence.BlockKey): the
// name a reader gave it, else its id. A queue addresses a unit by that key, so
// this is what maps a queue row onto a node in the rendered document.
function unitKeyOf(node: ContentNode): string {
  return node.name || node.id;
}

// blockNodesByUnitKey indexes the tree's translatable blocks by that key.
function blockNodesByUnitKey(tree: ContentTree | null): Map<string, ContentNode> {
  const out = new Map<string, ContentNode>();
  if (!tree) return out;
  const walk = (n: ContentNode) => {
    if (n.kind === "block") {
      const key = unitKeyOf(n);
      if (key && !out.has(key)) out.set(key, n);
    }
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
  /**
   * Open the document at one unit: the block with this key is marked, scrolled
   * into view, and named in the header. The key is the one a queue addresses a
   * unit by (`convergence.BlockKey`).
   */
  focusKey?: string | null;
  /**
   * Review state per unit key, drawn on each block as `data-review-state` and a
   * marker class. The reader sees where the decisions stand across the file
   * while reading it.
   */
  unitStates?: Record<string, string>;
  /** Label for the button that returns the reader where they came from. */
  backLabel?: string;
  /** Which side the document opens on: the source, or a target locale key. */
  side?: string;
}

// FilePreview is the desktop's project-content preview surface. It reuses the
// docs PreviewKit's DocumentViewer (Preview · Blocks · Stats · Download, with a
// source↔target toggle and annotation highlighting) so a project file renders
// exactly the way the documentation explorers render it — but driven by the
// desktop's full native engine via the InspectFileAnnotated binding rather than
// the WASM runtime.
//
// It calls InspectFileAnnotated so the tree carries the project's real
// terminology, voice-vocabulary and check overlays; the DocumentViewer's
// Annotations toggle highlights them on the rendered document. Committed targets
// from the project (translated/merged sibling files) ride along in the tree, so
// the source↔target toggle works whenever a translation exists.
//
// A host that arrives here to look at one unit passes `focusKey`, and the sheet
// opens at that block with the review states of the file's other units drawn
// alongside it. Every decision stays on the surface the reader came from: this
// one reads the document.
export function FilePreview({
  tabID,
  filePath,
  filename,
  onClose,
  entryPath,
  tree: presetTree,
  focusKey,
  unitStates,
  backLabel,
  side,
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

  // The written-back file behind the preview's File view. Catalog formats read
  // as a key table, and a reader looking at keys can ask what the file itself
  // will look like. Fetched only when asked for: it re-reads the source and
  // applies the stored targets.
  const [codeWanted, setCodeWanted] = useState(false);
  const codeLocale = side && side !== "source" ? side : "";
  const codeQuery = useQuery({
    queryKey: qk.writtenBackFile(tabID, filePath ?? "", codeLocale),
    enabled: codeWanted && !!filePath && !entryPath,
    queryFn: () => api.writtenBackFile(tabID, filePath!, codeLocale),
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

  // The focused unit's node, and the id every block is addressed by inside the
  // rendered document.
  const nodesByKey = useMemo(() => blockNodesByUnitKey(tree), [tree]);
  const focusNode = focusKey ? nodesByKey.get(focusKey) : undefined;
  const focusID = focusNode?.id;

  // Review state per block id, so one lookup answers for the whole document.
  const statesByBlockID = useMemo(() => {
    const out = new Map<string, string>();
    if (!unitStates) return out;
    for (const [key, node] of nodesByKey) {
      const state = unitStates[key];
      if (state) out.set(node.id, state);
    }
    return out;
  }, [nodesByKey, unitStates]);

  const blockAttrs = useCallback(
    (id: string): BlockAttrs | undefined => {
      const state = statesByBlockID.get(id);
      const focused = id === focusID;
      if (!state && !focused) return undefined;
      return {
        className: focused
          ? "ring-2 ring-primary/60 rounded-sm"
          : "underline decoration-dotted decoration-amber-500/70 underline-offset-4",
        "data-review-state": state,
        "data-review-focus": focused ? "true" : undefined,
      };
    },
    [statesByBlockID, focusID],
  );

  // Scroll the focused block into view once the document has rendered. The
  // sheet is its own scroll container, so the block is centred inside it.
  const bodyRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (!focusID || loading || error || !tree) return;
    const el = bodyRef.current?.querySelector<HTMLElement>(
      `[data-block-id="${CSS.escape(focusID)}"]`,
    );
    el?.scrollIntoView({ block: "center", behavior: "auto" });
  }, [focusID, loading, error, tree]);

  const focusState = focusKey ? unitStates?.[focusKey] : undefined;

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
            Structure, vocabulary and check annotations, and source &harr; target.
          </SheetDescription>
          {focusKey && (
            <div
              className="flex flex-wrap items-center gap-2 pt-1 text-xs"
              data-slot="file-preview-focus"
            >
              {backLabel && (
                <Button variant="outline" size="xs" onClick={onClose} data-slot="file-preview-back">
                  <ArrowLeft size={12} />
                  {backLabel}
                </Button>
              )}
              <span className="font-mono text-[11px] text-muted-foreground" translate="no">
                {focusKey}
              </span>
              <Badge variant="outline" className="text-[10px]">
                {focusState ?? t("awaiting review")}
              </Badge>
              {tree && !focusNode && (
                <span className="text-[11px] text-muted-foreground">
                  {t("This unit is not in the rendered document.")}
                </span>
              )}
            </div>
          )}
        </SheetHeader>

        <div ref={bodyRef} className="min-h-0 flex-1 overflow-auto px-4 pb-4">
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
              selectedBlockId={focusID}
              blockAttrs={blockAttrs}
              defaultSide={side}
              code={
                entryPath
                  ? undefined
                  : {
                      ...(codeQuery.data != null ? { text: codeQuery.data } : {}),
                      filename: codeLocale ? `${filename} (${codeLocale})` : filename,
                      loading: codeQuery.isFetching,
                      ...(codeQuery.error
                        ? {
                            error:
                              codeQuery.error instanceof Error
                                ? codeQuery.error.message
                                : String(codeQuery.error),
                          }
                        : {}),
                      onRequest: () => setCodeWanted(true),
                    }
              }
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

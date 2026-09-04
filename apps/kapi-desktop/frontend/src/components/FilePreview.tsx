import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FilePreview as PreviewSheet } from "@neokapi/ui-primitives/preview";
import type { ContentTree, ContentNode } from "@neokapi/ui-primitives/preview";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";

// collectMediaNodes walks the tree for media nodes that carry a resolvable URI
// (the image/audio/video readers emit the asset by URI). Each needs its bytes
// served to the frontend before the viewer can render it.
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

// FilePreview is the desktop's project-content preview surface. It binds the
// shared preview sheet (`@neokapi/ui-primitives/preview`) to the desktop's full
// native engine: InspectFileAnnotated supplies the tree, MediaDataURL serves the
// bytes behind each media node, and WrittenBackFile answers the keyed reading's
// File view. The sheet itself — header, states, focus row and the DocumentViewer
// body — is the same one the platform reads a file in.
//
// The tree carries the project's real terminology, voice-vocabulary and check
// overlays; the viewer's annotation highlighting draws them on the rendered
// document. Committed targets from the project (translated/merged sibling files)
// ride along, so the source↔target toggle works whenever a translation exists.
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

  return (
    <PreviewSheet
      open={!!filePath}
      onClose={onClose}
      filename={filename}
      description="Structure, vocabulary and check annotations, and source ↔ target."
      tree={tree}
      loading={loading}
      loadingLabel={t("Inspecting {filename}…", { filename })}
      error={error}
      focusKey={focusKey}
      unitStates={unitStates}
      backLabel={backLabel}
      viewer={{
        resolveMediaUrl: (node) => mediaUrls[node.id] ?? node.media?.uri,
        defaultSide: side,
        code: entryPath
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
            },
      }}
    />
  );
}

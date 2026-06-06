import { useEffect, useState } from "react";
import { Loader2, FileWarning } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@neokapi/ui-primitives";
import { DocumentViewer } from "@neokapi/kapi-lab";
import type { ContentTree } from "@neokapi/kapi-lab";
import { api } from "../hooks/useApi";

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
// terminology, brand-vocabulary and QA overlays; the DocumentViewer's
// Annotations toggle highlights them on the rendered document. Committed targets
// from the project (translated/merged sibling files) ride along in the tree, so
// the source↔target toggle works whenever a translation exists.
export function FilePreview({
  tabID,
  filePath,
  filename,
  onClose,
  tree: presetTree,
}: FilePreviewProps) {
  const [tree, setTree] = useState<ContentTree | null>(presetTree ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!filePath) return;
    if (presetTree) {
      setTree(presetTree);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    setTree(null);
    api
      .inspectFileAnnotated(tabID, filePath)
      .then((json) => {
        if (cancelled) return;
        if (!json) {
          setError("Preview is unavailable in this environment.");
          return;
        }
        setTree(JSON.parse(json) as ContentTree);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tabID, filePath, presetTree]);

  return (
    <Sheet open={!!filePath} onOpenChange={(open) => !open && onClose()}>
      <SheetContent side="right" className="w-full gap-3 sm:max-w-xl md:max-w-2xl lg:max-w-3xl">
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
          {!loading && !error && tree && <DocumentViewer tree={tree} filename={filename} />}
        </div>
      </SheetContent>
    </Sheet>
  );
}

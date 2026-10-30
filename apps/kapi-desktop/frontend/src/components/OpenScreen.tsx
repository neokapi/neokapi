import { useMemo } from "react";
import { Folder, FolderOpen, Store } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, EmptyState, SectionHeading, SimpleTooltip, When } from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { lastSegment } from "./WorkspaceProjectRow";
import type { WorkspaceHome } from "../types/api";

export interface OpenScreenProps {
  /** The workspace registry, whose projects are the recent folders. Null while it is read. */
  workspace: WorkspaceHome | null;
  /** Show the OS folder dialog and open the folder chosen. */
  onChooseFolder: () => void;
  /** Open a folder, a recipe or a file inside a folder, by path. */
  onOpenPath: (path: string) => void;
  /** Scaffold the KapiMart sample and open it. */
  onTrySample: () => void;
}

/** One recent folder: the project it holds and the recipe path that opens it. */
interface RecentFolder {
  key: string;
  name: string;
  dir: string;
  recipe: string;
  lastActive: string;
}

/**
 * What the app shows before a folder is open: the two ways in, and the folders
 * opened before. A folder dropped anywhere on the screen opens; a dropped file
 * opens the folder it is in.
 *
 * The window reports a drop natively, with the paths, to an element marked
 * `data-file-drop-target`; the webview's own drop event carries no path.
 */
export function OpenScreen({
  workspace,
  onChooseFolder,
  onOpenPath,
  onTrySample,
}: OpenScreenProps) {
  const shortenHome = useShortenHome();
  const recent = useMemo(() => recentFolders(workspace), [workspace]);

  useWailsEvent("files-dropped", (data) => {
    const path = droppedPath(data);
    if (path) onOpenPath(path);
  });

  return (
    <div
      data-slot="open-screen"
      data-file-drop-target
      className="flex h-full min-h-0 justify-center overflow-auto rounded-lg p-6 [&.file-drop-target-active]:bg-accent/40 [&.file-drop-target-active]:ring-2 [&.file-drop-target-active]:ring-primary/40"
    >
      <div className="w-full max-w-xl space-y-10 pt-[12vh]">
        <div className="space-y-6 text-center">
          <div className="space-y-2">
            <h1 className="text-xl font-semibold">{t("Drop a folder to see what it writes")}</h1>
            <p className="text-sm text-muted-foreground">
              {t(
                "Docs, help pages, UI text, marketing copy. kapi reads the files on this Mac and changes nothing until you ask.",
              )}
            </p>
          </div>
          <div className="flex justify-center gap-2">
            <Button data-slot="open-choose-folder" onClick={onChooseFolder}>
              <FolderOpen size={14} />
              {t("Choose a folder")}
            </Button>
            <Button
              data-slot="open-try-sample"
              data-testid="sample-kapimart"
              variant="outline"
              onClick={onTrySample}
            >
              <Store size={14} />
              {t("Try with KapiMart")}
            </Button>
          </div>
        </div>

        <section data-slot="open-recent" aria-labelledby="open-recent-heading">
          <div id="open-recent-heading" className="mb-1">
            <SectionHeading>{t("Recent folders")}</SectionHeading>
          </div>
          {recent.length > 0 ? (
            <ul className="divide-y divide-border/50">
              {recent.map((folder) => (
                <li key={folder.key}>
                  <SimpleTooltip
                    content={<span className="font-mono">{shortenHome(folder.dir)}</span>}
                  >
                    <button
                      type="button"
                      data-testid="open-recent-folder"
                      onClick={() => onOpenPath(folder.recipe)}
                      className="flex h-11 w-full min-w-0 items-center gap-3 rounded-md px-2 text-left hover:bg-accent"
                    >
                      <Folder aria-hidden size={16} className="shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate text-sm" translate="no">
                        {folder.name}
                      </span>
                      {folder.lastActive && (
                        <span className="shrink-0 text-[11px] text-muted-foreground">
                          <When iso={folder.lastActive} relative />
                        </span>
                      )}
                    </button>
                  </SimpleTooltip>
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState title={t("Recent folders appear here.")} />
          )}
        </section>
      </div>
    </div>
  );
}

/**
 * The folders the workspace has seen, newest first: one row for each project
 * with a checkout still on this machine. A project known only by its context
 * has no folder here to open.
 */
export function recentFolders(workspace: WorkspaceHome | null): RecentFolder[] {
  const out: RecentFolder[] = [];
  for (const project of workspace?.projects ?? []) {
    const live = project.checkouts.find((c) => !c.missing);
    if (!live) continue;
    out.push({
      key: project.key,
      name: project.name || lastSegment(live.path),
      dir: live.path,
      recipe: live.recipe,
      lastActive: project.last_active,
    });
  }
  return out.sort((a, b) => b.lastActive.localeCompare(a.lastActive));
}

/**
 * The path a native drop reported, for the first item dropped. The backend
 * resolves it: a folder opens, and a file opens the folder it is in.
 */
export function droppedPath(data: unknown): string | null {
  if (!Array.isArray(data)) return null;
  const first = data.find((p): p is string => typeof p === "string" && p !== "");
  return first ?? null;
}

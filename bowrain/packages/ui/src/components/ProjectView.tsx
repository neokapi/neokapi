import {
  Badge,
  Button,
  Card,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  useIsMobile,
} from "@neokapi/ui-primitives";
import { useState, useRef, useCallback, useMemo } from "react";
import type { ProjectInfo, CollectionInfo, StreamInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { useStream } from "../context/StreamContext";
import { OpenInDesktop } from "./OpenInDesktop";
import { CollectionRail } from "./CollectionRail";
import { commonItemBase, itemDisplayPath, relativeItemName } from "./collections/itemBase";
import { ProjectTypeBadge } from "./ProjectTypeBadge";
import { FormattedFileName } from "./FormattedFileName";
import { t } from "@neokapi/i18n-react/runtime";
import { FilePreview, type ItemPreviewBinding } from "./FilePreview";
import { useInContextItems } from "../hooks/useInContextItems";
import { ListCapRow } from "./ListCapRow";
import {
  ArrowRight,
  X,
  Package,
  Plug,
  Upload,
  MonitorPlay,
  MoreHorizontal,
  Pencil,
  Settings,
  Trash2,
  Users,
} from "./icons";

/** Hard cap on rendered file/item rows; larger collections show a ListCapRow. */
const MAX_ITEM_ROWS = 500;

/**
 * The noun a collection calls its items, in the form `count` needs.
 *
 * A collection's `item_label` arrives from the server as a bare singular
 * ("file", "page", "post", …), and appending an "s" to it is a rule that holds
 * in one language. Each known label is its own message here instead, so a
 * translator supplies both forms and a language with more than two gets the
 * rest. An unrecognised label falls back to the generic noun rather than being
 * inflected by a rule that may not apply to it.
 */
function itemNoun(label: string, count: number): string {
  switch (label) {
    case "page":
      return t("{count, plural, one {page} other {pages}}", { count });
    case "post":
      return t("{count, plural, one {post} other {posts}}", { count });
    case "document":
      return t("{count, plural, one {document} other {documents}}", { count });
    case "file":
      return t("{count, plural, one {file} other {files}}", { count });
    default:
      return t("{count, plural, one {item} other {items}}", { count });
  }
}

export interface ProjectViewProps {
  project: ProjectInfo;
  onBack: () => void;
  /** Open one file's editor surfaces. The item name is the coordinate the
   *  server addresses an item by, so it is what travels in the route.
   *  Ignored when `preview` is set. */
  onOpenFile: (itemName: string) => void;
  /**
   * Given, a row opens the file's preview instead of an editor, and the editors
   * are reached from the preview's own actions.
   */
  preview?: ItemPreviewBinding;
  /** Upload files via adapter. Web apps pass File objects; desktop passes file paths. */
  onUploadFiles: (files: File[]) => void;
  onRemoveFile: (fileName: string) => void;
  onOpenMemory?: () => void;
  onOpenTerms?: () => void;
  /** When set, shows "Open in Bowrain Desktop" banner with deep link. */
  serverMode?: { serverURL: string; workspaceSlug: string };
  /**
   * The collection being read, and how to change it. Given, the selection is
   * the consumer's — a search param, so a reader can link to a collection and
   * Back returns to the one before. Without it the view holds the selection
   * itself and a reload lands on the first collection again.
   */
  collection?: { id: string | null; onSelect: (id: string) => void };
  /** Collection callbacks */
  onCreateCollection?: () => void;
  onEditCollection?: (collection: CollectionInfo) => void;
  onDeleteCollection?: (id: string) => void;
  onUploadToCollection?: (collectionId: string, files: File[]) => void;
  /** Project actions */
  onEditProject?: () => void;
  onArchiveProject?: () => void;
  /** Stream callbacks */
  onCreateStream?: () => void;
  onEditStream?: (stream: StreamInfo) => void;
  onMergeStream?: (streamName: string) => void;
  onDiffStream?: (streamName: string) => void;
  onDeleteStream?: (streamName: string) => void;
  /** Open project member management. */
  onManageMembers?: () => void;
  /** Navigate to the project's overview. */
  onOpenDashboard?: () => void;
  /** Navigate to project settings page. */
  onOpenSettings?: () => void;
}

export function ProjectView({
  project,
  onBack: _onBack,
  onOpenFile,
  preview,
  collection,
  onUploadFiles,
  onRemoveFile,
  onOpenMemory,
  onOpenTerms,
  serverMode,
  onCreateCollection,
  onEditCollection,
  onDeleteCollection,
  onUploadToCollection,
  onEditProject,
  onArchiveProject,
  onCreateStream: _onCreateStream,
  onEditStream: _onEditStream,
  onMergeStream: _onMergeStream,
  onDiffStream: _onDiffStream,
  onDeleteStream: _onDeleteStream,
  onManageMembers,
  onOpenDashboard,
  onOpenSettings,
}: ProjectViewProps) {
  const { getDisplayName } = useLocales();
  const isMobile = useIsMobile();
  const { activeStream: _activeStream, setActiveStream: _setActiveStream } = useStream();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);
  const [ownCollectionId, setOwnCollectionId] = useState<string | null>(null);

  // Register breadcrumb in the top bar area

  const collections = project.collections ?? [];
  const hasCollections = collections.length > 0;
  // One collection and no way to add another is not a navigation: the items
  // header already names where they are.
  const showRail = collections.length > 1 || (hasCollections && !!onCreateCollection);

  // Which collection is being read. The consumer owns it when it binds one (so
  // it lives in the URL); otherwise this view remembers it for the session.
  const selectedCollectionId = collection ? collection.id : ownCollectionId;
  const selectCollection = collection ? collection.onSelect : setOwnCollectionId;
  // A collection named in the URL that this project does not hold — a stale
  // link, or a collection since removed — reads as the first one rather than as
  // an empty project.
  const effectiveCollectionId =
    (selectedCollectionId && collections.some((c) => c.id === selectedCollectionId)
      ? selectedCollectionId
      : null) ??
    collections[0]?.id ??
    null;
  const activeCollection = collections.find((c) => c.id === effectiveCollectionId) ?? null;

  // Filter items by active collection (if collections exist)
  const allItems = project.items ?? [];
  const items =
    hasCollections && effectiveCollectionId
      ? allItems.filter((item) => item.collection_id === effectiveCollectionId)
      : allItems;

  // Hard render cap: very large collections (thousands of files) should not
  // mount thousands of table rows. The cap is surfaced honestly via ListCapRow.
  const visibleItems = useMemo(() => items.slice(0, MAX_ITEM_ROWS), [items]);

  // What every item in this collection shares — its `base:` and whatever lies
  // below it — stated once above the list instead of on every row. Computed
  // over the whole collection, not the rendered page, so it does not move when
  // the cap bites.
  // Over the paths the rows SHOW, not over their names: a base computed over
  // names prefixes none of the source paths beside them, and every row would
  // read whole.
  const itemBase = useMemo(
    () => commonItemBase(items.map((i) => itemDisplayPath(i.name, i.source_path))),
    [items],
  );

  // A row reads the file; an editor is entered deliberately. Without a preview
  // binding the row keeps its older behaviour and opens the editor directly.
  const openItem = preview ? preview.onOpen : onOpenFile;
  const previewItem = preview ? allItems.find((i) => i.name === preview.itemName) : undefined;
  // Which of these rows have a component published beside them. Resolved once
  // for the collection, so the row can say so and the preview can offer the
  // reading only where it leads somewhere.
  const inContext = useInContextItems(project.id, activeCollection?.id, activeCollection?.preview);

  const totalBlocks = items.reduce((sum, f) => sum + f.block_count, 0);
  const totalWords = items.reduce((sum, f) => sum + f.word_count, 0);

  // Is source mutation (upload/add/delete) allowed here? The server folds both
  // signals into `editable`: a Managed collection is editable; a connector-
  // sourced one (its own connector, or a project bound to a source connector —
  // kapi push / GitHub App / git) is not, even when its kind is "uploaded". Fall
  // back to the legacy kind check for older responses that omit `editable`.
  const projectEditable = project.editable !== false;
  const canUpload = activeCollection
    ? (activeCollection.editable ?? activeCollection.kind === "uploaded")
    : projectEditable;
  const itemLabel = activeCollection?.item_label ?? "file";
  const itemLabelPlural = itemNoun(itemLabel, items.length);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      if (e.dataTransfer.files.length > 0) {
        const files = Array.from(e.dataTransfer.files);
        if (onUploadToCollection && effectiveCollectionId) {
          onUploadToCollection(effectiveCollectionId, files);
        } else {
          onUploadFiles(files);
        }
      }
    },
    [onUploadFiles, onUploadToCollection, effectiveCollectionId],
  );

  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files && e.target.files.length > 0) {
        const files = Array.from(e.target.files);
        if (onUploadToCollection && effectiveCollectionId) {
          onUploadToCollection(effectiveCollectionId, files);
        } else {
          onUploadFiles(files);
        }
        e.target.value = "";
      }
    },
    [onUploadFiles, onUploadToCollection, effectiveCollectionId],
  );

  return (
    <div className="flex-1 min-h-0 overflow-auto">
      {serverMode && (
        <OpenInDesktop
          projectId={project.id}
          serverURL={serverMode.serverURL}
          workspaceSlug={serverMode.workspaceSlug}
        />
      )}

      {/* Project overview card */}
      <Card className={isMobile ? "p-4 mb-3" : "p-6 mb-4"}>
        <div
          className={
            isMobile ? "flex flex-col gap-3 mb-4" : "flex items-center justify-between mb-6"
          }
        >
          <div>
            <div className="flex items-center gap-2">
              {/* The project's name is on screen three times now — the trail,
                  the panel that holds its sections, and here — so the page's
                  own title needs a hook that says which one it is. */}
              <h2
                data-testid="project-title"
                className={isMobile ? "text-lg font-semibold" : "text-xl font-semibold"}
              >
                {project.name}
              </h2>
              {project.type && <ProjectTypeBadge type={project.type} />}
            </div>
            <p className="text-[13px] text-muted-foreground mt-1">
              {getDisplayName(project.default_source_language)}{" "}
              <ArrowRight className="w-3.5 h-3.5 inline-block" />{" "}
              {project.target_languages.map((l) => getDisplayName(l)).join(", ")}
            </p>
          </div>
          <div className="flex gap-2">
            {onOpenDashboard && (
              <Button
                variant="ghost"
                size="sm"
                onClick={onOpenDashboard}
                data-testid="open-dashboard-btn"
              >
                Overview
              </Button>
            )}
            {onOpenTerms && (
              <Button variant="ghost" size="sm" onClick={onOpenTerms} data-testid="open-terms-btn">
                Terminology
              </Button>
            )}
            {onOpenMemory && (
              <Button variant="ghost" size="sm" onClick={onOpenMemory} data-testid="open-tm-btn">
                Content memory
              </Button>
            )}
            {(onEditProject || onArchiveProject || onManageMembers || onOpenSettings) && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors cursor-pointer bg-transparent border-none">
                    <MoreHorizontal className="w-4 h-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-[150px]">
                  {onOpenSettings && (
                    <DropdownMenuItem
                      onClick={onOpenSettings}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Settings className="w-3.5 h-3.5" /> Settings
                    </DropdownMenuItem>
                  )}
                  {onManageMembers && (
                    <DropdownMenuItem
                      onClick={onManageMembers}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Users className="w-3.5 h-3.5" /> Members
                    </DropdownMenuItem>
                  )}
                  {onEditProject && (
                    <DropdownMenuItem
                      onClick={onEditProject}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Pencil className="w-3.5 h-3.5" /> Edit project
                    </DropdownMenuItem>
                  )}
                  {onArchiveProject && (
                    <>
                      {(onEditProject || onOpenSettings) && <DropdownMenuSeparator />}
                      <DropdownMenuItem
                        onClick={onArchiveProject}
                        className="flex items-center gap-2 text-sm text-destructive"
                      >
                        <Trash2 className="w-3.5 h-3.5" /> Archive
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>

        <div className="flex gap-3">
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>
              {items.length}
            </div>
            <div className="text-xs text-muted-foreground capitalize">{itemLabelPlural}</div>
          </div>
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>
              {totalBlocks}
            </div>
            <div className="text-xs text-muted-foreground">Blocks</div>
          </div>
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>
              {totalWords}
            </div>
            <div className="text-xs text-muted-foreground">Words</div>
          </div>
        </div>
      </Card>

      {/* The collections beside their content: the rail states the project's
          shape once, and the items stay the width of the card. */}
      <Card className={isMobile ? "p-4" : "p-6"}>
        <div
          className={
            showRail
              ? "grid gap-6 lg:grid-cols-[minmax(11rem,15rem)_minmax(0,1fr)] lg:items-start"
              : undefined
          }
        >
          {showRail && (
            <CollectionRail
              collections={collections}
              activeCollectionId={effectiveCollectionId}
              onSelectCollection={selectCollection}
              onCreateCollection={onCreateCollection}
              onEditCollection={onEditCollection}
              onDeleteCollection={onDeleteCollection}
            />
          )}

          <div className="min-w-0">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h3 className={isMobile ? "text-base font-semibold" : "text-lg font-semibold"}>
                  <span className="capitalize">{itemLabelPlural}</span>
                </h3>
                <p className="text-[13px] text-muted-foreground mt-1">
                  {items.length} {itemLabelPlural}
                  {activeCollection && !activeCollection.is_default
                    ? ` in ${activeCollection.name}`
                    : " in project"}
                </p>
                {/* What every row shares, said once. The rows then carry only
                    what tells them apart; each keeps its full name on hover. */}
                {itemBase && (
                  <p
                    className="mt-0.5 break-all font-mono text-[12px] text-muted-foreground/70"
                    title={itemBase}
                    translate="no"
                    data-testid="item-base"
                  >
                    {itemBase}
                  </p>
                )}
              </div>

              {/* Upload button — only for uploaded collections */}
              {canUpload && (
                <div>
                  <input
                    ref={inputRef}
                    type="file"
                    multiple
                    onChange={handleFileInputChange}
                    className="hidden"
                  />
                  <Button
                    size="sm"
                    onClick={() => inputRef.current?.click()}
                    data-testid="add-files-btn"
                  >
                    <Upload className="w-3.5 h-3.5 mr-1.5" />
                    <span className="capitalize">Add {itemNoun(itemLabel, 0)}</span>
                  </Button>
                </div>
              )}

              {/* Connected badge — for connected collections */}
              {!canUpload && activeCollection && (
                <Badge variant="secondary" className="gap-1.5">
                  <Plug className="w-3 h-3" />
                  Connected
                </Badge>
              )}
            </div>

            {/* Drop zone — only for uploaded collections */}
            {canUpload && !isMobile && (
              <div
                className={`flex flex-col items-center justify-center gap-2 p-8 mb-6 rounded-lg border border-dashed border-border transition-all ${dragOver ? "ring-2 ring-primary bg-accent/30" : "bg-accent/10"}`}
                onDragOver={(e) => {
                  e.preventDefault();
                  setDragOver(true);
                }}
                onDragLeave={() => setDragOver(false)}
                onDrop={handleDrop}
                data-testid="file-drop-zone"
              >
                <Package className="w-8 h-8 text-muted-foreground opacity-30" />
                <span className="text-muted-foreground text-[13px]">
                  Drag and drop {itemLabelPlural} here to add them
                  {activeCollection && !activeCollection.is_default
                    ? ` to ${activeCollection.name}`
                    : " to the project"}
                </span>
              </div>
            )}

            {/* Connected collection info panel */}
            {!canUpload && activeCollection && (
              <div className="flex items-center gap-3 p-4 mb-6 rounded-lg border border-border/50 bg-accent/10">
                <Plug className="w-5 h-5 text-muted-foreground shrink-0" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-foreground">
                    This collection syncs content from an external source.
                  </p>
                  <p className="text-[12px] text-muted-foreground mt-0.5">
                    Items are managed by the connected integration and cannot be uploaded manually.
                  </p>
                </div>
              </div>
            )}

            {/* Items table */}
            {items.length > 0 && (
              <div className="overflow-x-auto">
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="border-b border-border">
                      <th
                        className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-left text-sm font-medium text-muted-foreground`}
                      >
                        <span className="capitalize">{itemLabel}</span>
                      </th>
                      {!isMobile && (
                        <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                          Format
                        </th>
                      )}
                      <th
                        className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-right text-sm font-medium text-muted-foreground`}
                      >
                        Blocks
                      </th>
                      {!isMobile && (
                        <th className="px-4 py-2.5 text-right text-sm font-medium text-muted-foreground">
                          Words
                        </th>
                      )}
                      {canUpload && (
                        <th
                          className={`${isMobile ? "px-1 w-10" : "px-4 w-20"} py-2.5 text-sm font-medium text-muted-foreground`}
                        ></th>
                      )}
                    </tr>
                  </thead>
                  <tbody>
                    {visibleItems.map((f) => (
                      // The whole row is the pointer target for reading the file;
                      // the name inside stays a button, so the row is reachable by
                      // keyboard without nesting focus inside a focusable row.
                      <tr
                        key={f.name}
                        className={`border-b border-border/50 transition-colors hover:bg-accent/50 ${preview ? "cursor-pointer" : ""}`}
                        onClick={preview ? () => openItem(f.name) : undefined}
                        data-testid={`file-row-${f.name}`}
                      >
                        <td className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-sm`}>
                          <button
                            onClick={() => openItem(f.name)}
                            title={f.name}
                            className="bg-transparent border-none text-primary cursor-pointer text-sm p-0 hover:underline inline-flex items-center gap-1.5 text-left break-all"
                            data-testid={`open-file-${f.name}`}
                          >
                            <FormattedFileName
                              name={relativeItemName(
                                itemDisplayPath(f.name, f.source_path),
                                itemBase,
                              )}
                              format={f.format}
                            />
                          </button>
                          {/* Marked, never dimmed: a row without a story is not
                              worse content, it is content with no component
                              published beside it. */}
                          {inContext.enabled && inContext.has(f.source_path) && (
                            <MonitorPlay
                              className="ml-1.5 inline-block size-3.5 shrink-0 align-text-bottom text-muted-foreground"
                              aria-label="Can be read in context"
                              data-testid="file-in-context"
                            />
                          )}
                        </td>
                        {!isMobile && (
                          <td className="px-4 py-2.5 text-sm">
                            <Badge variant="secondary">{f.format}</Badge>
                          </td>
                        )}
                        <td
                          className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-sm text-muted-foreground text-right`}
                        >
                          {f.block_count}
                        </td>
                        {!isMobile && (
                          <td className="px-4 py-2.5 text-sm text-muted-foreground text-right">
                            {f.word_count}
                          </td>
                        )}
                        {canUpload && (
                          <td className={`${isMobile ? "px-1" : "px-4"} py-2.5 text-sm text-right`}>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                onRemoveFile(f.name);
                              }}
                              className="bg-transparent border-none text-muted-foreground cursor-pointer px-2 py-1 rounded hover:text-destructive transition-colors"
                              data-testid={`remove-file-${f.name}`}
                            >
                              <X className="w-3.5 h-3.5" />
                            </button>
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
                <ListCapRow
                  shown={visibleItems.length}
                  total={items.length}
                  noun={itemLabelPlural}
                  hint="Open the project in the desktop app or CLI for the full listing."
                />
              </div>
            )}

            {/* Empty state */}
            {items.length === 0 && !canUpload && (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Plug className="w-10 h-10 text-muted-foreground/30 mb-3" />
                <p className="text-sm text-muted-foreground">No {itemLabelPlural} synced yet</p>
                <p className="text-[12px] text-muted-foreground/60 mt-1">
                  Content will appear here when the connected source syncs
                </p>
              </div>
            )}
          </div>
        </div>
      </Card>

      {preview && (
        <FilePreview
          projectId={project.id}
          itemName={preview.itemName}
          format={previewItem?.format}
          sourcePath={previewItem?.source_path}
          targetLocales={preview.targetLocales ?? project.target_languages}
          sourceLocale={preview.sourceLocale ?? project.default_source_language}
          onClose={preview.onClose}
          onOpenTranslate={preview.onOpenTranslate}
          onOpenReview={preview.onOpenReview}
          onOpenPreProcess={preview.onOpenPreProcess}
          // Where THIS collection publishes its components. Per collection
          // because a repository publishes one host per surface it ships: the
          // desktop app's components and the web app's are two Storybooks, and
          // a project-wide URL would offer each collection the other's.
          preview={activeCollection?.preview}
          collectionId={activeCollection?.id}
          hasStory={inContext.has(previewItem?.source_path)}
        />
      )}
    </div>
  );
}

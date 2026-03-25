import { useState, useRef, useCallback, useMemo } from "react";
import type { ProjectInfo, CollectionInfo, StreamInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { useIsMobile } from "../hooks/useIsMobile";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { useStream } from "../context/StreamContext";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { Card } from "./ui/card";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "./ui/dropdown-menu";
import { OpenInDesktop } from "./OpenInDesktop";
import { Switch } from "./ui/switch";
import { CollectionTabs } from "./CollectionTabs";
import {
  ArrowLeft,
  ArrowRight,
  Globe,
  FileCode,
  FileJson,
  FileText,
  FileType,
  MessageSquare,
  FileSpreadsheet,
  X,
  Lock,
  Package,
  Plug,
  Upload,
  MoreHorizontal,
  Activity,
  Pencil,
  Trash2,
  Users,
} from "./icons";

export interface ProjectViewProps {
  project: ProjectInfo;
  onBack: () => void;
  onOpenFile: (itemId: string) => void;
  /** Upload files via adapter. Web apps pass File objects; desktop passes file paths. */
  onUploadFiles: (files: File[]) => void;
  onRemoveFile: (fileName: string) => void;
  onOpenTM?: () => void;
  onOpenTerms?: () => void;
  /** When set, shows "Open in Bowrain Desktop" banner with deep link. */
  serverMode?: { serverURL: string; workspaceSlug: string };
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
  /** Navigate to the translation dashboard for this project. */
  onOpenDashboard?: () => void;
  /** Toggle project visibility on the Pulse public dashboard. */
  onTogglePulseVisibility?: () => void;
}

export function ProjectView({
  project,
  onBack,
  onOpenFile,
  onUploadFiles,
  onRemoveFile,
  onOpenTM,
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
  onTogglePulseVisibility,
}: ProjectViewProps) {
  const { getDisplayName } = useLocales();
  const isMobile = useIsMobile();
  const { activeStream: _activeStream, setActiveStream: _setActiveStream } = useStream();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);
  const [activeCollectionId, setActiveCollectionId] = useState<string | null>(null);

  // Register breadcrumb in the top bar area
  const breadcrumbNode = useMemo(
    () => (
      <button
        onClick={onBack}
        data-testid="back-to-projects"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        <ArrowLeft className="w-3.5 h-3.5" /> Projects
      </button>
    ),
    [onBack],
  );
  useSetBreadcrumb(breadcrumbNode);

  const collections = project.collections ?? [];
  const hasCollections = collections.length > 0;

  // Determine which collection is active
  const effectiveCollectionId = activeCollectionId ?? collections[0]?.id ?? null;
  const activeCollection = collections.find((c) => c.id === effectiveCollectionId) ?? null;

  // Filter items by active collection (if collections exist)
  const allItems = project.items ?? [];
  const items =
    hasCollections && effectiveCollectionId
      ? allItems.filter((item) => item.collection_id === effectiveCollectionId)
      : allItems;

  const totalBlocks = items.reduce((sum, f) => sum + f.block_count, 0);
  const totalWords = items.reduce((sum, f) => sum + f.word_count, 0);

  // Is upload allowed for the active collection?
  const canUpload = !activeCollection || activeCollection.kind === "uploaded";
  const itemLabel = activeCollection?.item_label ?? "file";
  const itemLabelPlural = items.length === 1 ? itemLabel : `${itemLabel}s`;

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

  const formatIcon = (format: string) => {
    const cls = "w-4 h-4 inline-block align-text-bottom";
    const icons: Record<string, React.ReactNode> = {
      html: <Globe className={cls} />,
      xml: <FileCode className={cls} />,
      json: <FileJson className={cls} />,
      yaml: <FileText className={cls} />,
      plaintext: <FileType className={cls} />,
      po: <MessageSquare className={cls} />,
      properties: <Lock className={cls} />,
      markdown: <FileText className={cls} />,
      csv: <FileSpreadsheet className={cls} />,
      xliff: <ArrowRight className={cls} />,
      xliff2: <ArrowRight className={cls} />,
    };
    return icons[format] || <FileCode className={cls} />;
  };

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
            <h2 className={isMobile ? "text-lg font-semibold" : "text-xl font-semibold"}>
              {project.name}
            </h2>
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
                Dashboard
              </Button>
            )}
            {onOpenTerms && (
              <Button variant="ghost" size="sm" onClick={onOpenTerms} data-testid="open-terms-btn">
                Terminology
              </Button>
            )}
            {onOpenTM && (
              <Button variant="ghost" size="sm" onClick={onOpenTM} data-testid="open-tm-btn">
                Translation Memory
              </Button>
            )}
            {(onEditProject || onArchiveProject || onManageMembers) && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors cursor-pointer bg-transparent border-none">
                    <MoreHorizontal className="w-4 h-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-[150px]">
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
                      {onEditProject && <DropdownMenuSeparator />}
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

      {onTogglePulseVisibility && (
        <div className="flex items-center justify-between rounded-lg border border-border/50 px-4 py-3">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-muted-foreground" />
            <div>
              <p className="text-sm font-medium">Show on Pulse</p>
              <p className="text-xs text-muted-foreground">
                Make this project visible on the public dashboard
              </p>
            </div>
          </div>
          <Switch
            checked={project.dashboard_visibility === "public"}
            onCheckedChange={onTogglePulseVisibility}
            aria-label="Toggle Pulse visibility"
          />
        </div>
      )}

      {/* Content card with collection tabs */}
      <Card className={isMobile ? "p-4" : "p-6"}>
        {/* Collection tabs */}
        {hasCollections && (
          <CollectionTabs
            collections={collections}
            activeCollectionId={effectiveCollectionId}
            onSelectCollection={setActiveCollectionId}
            onCreateCollection={onCreateCollection}
            onEditCollection={onEditCollection}
            onDeleteCollection={onDeleteCollection}
          />
        )}

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
                Add {itemLabel === "file" ? "Files" : itemLabel + "s"}
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
                {items.map((f) => (
                  <tr
                    key={f.name}
                    className="border-b border-border/50 transition-colors hover:bg-accent/50"
                    data-testid={`file-row-${f.name}`}
                  >
                    <td className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-sm`}>
                      <button
                        onClick={() => onOpenFile(f.id)}
                        className="bg-transparent border-none text-primary cursor-pointer text-sm p-0 hover:underline inline-flex items-center gap-1.5 text-left break-all"
                        data-testid={`open-file-${f.name}`}
                      >
                        {formatIcon(f.format)} {f.name}
                      </button>
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
      </Card>
    </div>
  );
}

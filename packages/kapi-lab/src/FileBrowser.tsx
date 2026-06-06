import React, { useMemo, useState } from "react";
import { LayoutGrid, List as ListIcon } from "lucide-react";
import { Badge, ToggleGroup, ToggleGroupItem, cn } from "@neokapi/ui-primitives";
import FormatPreview from "./FormatPreview";
import DocumentViewer from "./DocumentViewer";
import { FileIcon, fileType } from "./fileTypes";
import { formatBytes } from "./download";
import type { ContentTree } from "./types";

// FileBrowser — show many files across formats in list or grid views (a toggle).
// Each item is a small FormatPreview thumbnail + name / format / size; selecting
// one opens it in a DocumentViewer (inline by default, or via the onOpen
// callback so a host can route it elsewhere). Works across mixed file types.

export interface BrowserFile {
  /** Stable id (defaults to filename when omitted). */
  id?: string;
  filename: string;
  /** The engine output for this file (from `kapi inspect`). */
  tree: ContentTree;
  /** Original bytes, enabling Download in the viewer (optional). */
  bytes?: Uint8Array | null;
}

export interface FileBrowserProps {
  files: BrowserFile[];
  /** Initial layout (default "grid"). */
  defaultView?: "list" | "grid";
  /**
   * Called when a file is selected. When provided, the browser does NOT open an
   * inline viewer — the host owns presentation. When omitted, the browser opens
   * the selected file inline below the grid/list.
   */
  onOpen?: (file: BrowserFile) => void;
  className?: string;
}

function fileId(f: BrowserFile): string {
  return f.id ?? f.filename;
}

export default function FileBrowser({
  files,
  defaultView = "grid",
  onOpen,
  className,
}: FileBrowserProps): React.ReactElement {
  const [view, setView] = useState<"list" | "grid">(defaultView);
  const [openId, setOpenId] = useState<string | null>(null);

  const opened = useMemo(
    () => (onOpen ? null : (files.find((f) => fileId(f) === openId) ?? null)),
    [files, openId, onOpen],
  );

  const select = (f: BrowserFile) => {
    if (onOpen) onOpen(f);
    else setOpenId((cur) => (cur === fileId(f) ? null : fileId(f)));
  };

  return (
    <div className={cn("kapi-reference flex flex-col gap-3", className)}>
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          {files.length} file{files.length === 1 ? "" : "s"}
        </span>
        <ToggleGroup
          type="single"
          size="sm"
          value={view}
          onValueChange={(v) => v && setView(v as "list" | "grid")}
          aria-label="View"
        >
          <ToggleGroupItem value="grid" aria-label="Grid view">
            <LayoutGrid className="size-4" />
          </ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label="List view">
            <ListIcon className="size-4" />
          </ToggleGroupItem>
        </ToggleGroup>
      </div>

      {view === "grid" ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {files.map((f) => (
            <GridCard
              key={fileId(f)}
              file={f}
              active={openId === fileId(f)}
              onClick={() => select(f)}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col rounded-lg border border-border">
          {files.map((f) => (
            <ListRow
              key={fileId(f)}
              file={f}
              active={openId === fileId(f)}
              onClick={() => select(f)}
            />
          ))}
        </div>
      )}

      {opened && (
        <DocumentViewer tree={opened.tree} filename={opened.filename} bytes={opened.bytes} />
      )}
    </div>
  );
}

function Meta({ file }: { file: BrowserFile }): React.ReactElement {
  const ft = fileType(file.filename);
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <FileIcon filename={file.filename} size={14} />
      <span className="truncate font-mono text-xs" title={file.filename}>
        {file.filename}
      </span>
      <Badge
        variant="outline"
        className={cn("ml-auto border-current/35 text-[10px]", ft.colorClass)}
      >
        {ft.label}
      </Badge>
    </div>
  );
}

function GridCard({
  file,
  active,
  onClick,
}: {
  file: BrowserFile;
  active: boolean;
  onClick: () => void;
}): React.ReactElement {
  const ft = fileType(file.filename);
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        // One themed border that frames the preview itself (no inner box, no
        // bare `border` — which falls back to currentColor/black outside a
        // Tailwind-preflight host). The preview fills the frame.
        "group relative block aspect-[4/3] overflow-hidden rounded-lg border border-border bg-background text-left transition-colors hover:border-primary/60",
        active && "border-primary ring-1 ring-primary/40",
      )}
    >
      {/* Preview fills 100% of the card — no inner box, no padding gap. */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <FormatPreview tree={file.tree} annotations={false} flush className="h-full w-full" />
      </div>
      {/* File metadata as a blurred overlay pinned to the bottom of the preview. */}
      <div className="absolute inset-x-0 bottom-0 flex items-center gap-1.5 border-t border-border/40 bg-card/70 px-2 py-1.5 backdrop-blur-md">
        <FileIcon filename={file.filename} size={14} />
        <span className="truncate font-mono text-xs" title={file.filename}>
          {file.filename}
        </span>
        <Badge
          variant="outline"
          className={cn("ml-auto shrink-0 border-current/35 text-[10px]", ft.colorClass)}
        >
          {ft.label}
        </Badge>
        {file.bytes && (
          <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">
            {formatBytes(file.bytes.length)}
          </span>
        )}
      </div>
    </button>
  );
}

function ListRow({
  file,
  active,
  onClick,
}: {
  file: BrowserFile;
  active: boolean;
  onClick: () => void;
}): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center gap-3 border-b border-border px-3 py-2 text-left transition-colors last:border-b-0 hover:bg-muted/40",
        active && "bg-muted/60",
      )}
    >
      <div className="pointer-events-none h-12 w-20 shrink-0 overflow-hidden rounded border border-border bg-background p-0.5">
        <FormatPreview
          tree={file.tree}
          annotations={false}
          className="scale-[0.5] origin-top-left"
        />
      </div>
      <div className="min-w-0 flex-1">
        <Meta file={file} />
      </div>
      {file.bytes && (
        <span className="text-xs tabular-nums text-muted-foreground">
          {formatBytes(file.bytes.length)}
        </span>
      )}
    </button>
  );
}

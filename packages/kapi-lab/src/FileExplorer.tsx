import React, { useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, Download, FolderOpen, Plus, Regex, Upload } from "lucide-react";
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDeleteButton,
  EmptyState,
  GlobInput,
  ScrollArea,
  Separator,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from "@neokapi/ui-primitives";
import { FileIcon, fileType } from "@neokapi/ui-primitives/preview";
import { downloadBytes, formatBytes } from "@neokapi/ui-primitives/preview";
import { matchGlob } from "./glob";
import { resolveSelection, selectionSummary } from "./fileLibrary";
import type { FileLibrary, FileSelection, LibFile } from "./fileLibrary";
import { SAMPLES } from "./samples";

export interface FileExplorerProps {
  library: FileLibrary;
  selection: FileSelection;
  onSelectionChange: (sel: FileSelection) => void;
  /** Allow multi-select + glob selection (default true). */
  multiple?: boolean;
  /** Samples offered in the "add" shelf (default: all). */
  sampleIds?: string[];
  /** Preview a file (e.g. open it in the output viewer). */
  onPreview?: (file: LibFile) => void;
  className?: string;
}

interface TreeDir {
  name: string;
  path: string;
  dirs: TreeDir[];
  files: LibFile[];
}

function buildTree(files: LibFile[]): TreeDir {
  const root: TreeDir = { name: "", path: "", dirs: [], files: [] };
  for (const f of files) {
    const parts = f.path.split("/");
    let cur = root;
    for (let i = 0; i < parts.length - 1; i++) {
      const segPath = parts.slice(0, i + 1).join("/");
      let next = cur.dirs.find((d) => d.path === segPath);
      if (!next) {
        next = { name: parts[i], path: segPath, dirs: [], files: [] };
        cur.dirs.push(next);
      }
      cur = next;
    }
    cur.files.push(f);
  }
  const sortDir = (d: TreeDir) => {
    d.dirs.sort((a, b) => a.name.localeCompare(b.name));
    d.files.sort((a, b) => a.name.localeCompare(b.name));
    d.dirs.forEach(sortDir);
  };
  sortDir(root);
  return root;
}

function countFiles(dir: TreeDir): number {
  return dir.files.length + dir.dirs.reduce((n, d) => n + countFiles(d), 0);
}

// FileLabel renders a filename with its dot-extension slightly dimmed, so the
// base name reads first and the format suffix recedes.
function FileLabel({
  name,
  title,
  className,
}: {
  name: string;
  title?: string;
  className?: string;
}): React.ReactElement {
  const dot = name.lastIndexOf(".");
  const base = dot > 0 ? name.slice(0, dot) : name;
  const ext = dot > 0 ? name.slice(dot) : "";
  return (
    <span className={cn("truncate font-mono", className)} title={title ?? name}>
      {base}
      {ext && <span className="text-muted-foreground/60">{ext}</span>}
    </span>
  );
}

const ORIGIN_VARIANT: Record<LibFile["origin"], "secondary" | "outline"> = {
  sample: "secondary",
  upload: "outline",
  output: "outline",
};

// FileExplorer is the full file-management surface: pick one file, many files,
// or a glob across the bundled samples, your uploads and any pipeline outputs.
// Files carry type icons, sizes and an origin badge; files and folders can be
// downloaded or deleted; outputs are flagged so it's clear what the engine
// produced. Built entirely from the shared shadcn primitives.
export default function FileExplorer({
  library,
  selection,
  onSelectionChange,
  multiple = true,
  sampleIds,
  onPreview,
  className,
}: FileExplorerProps): React.ReactElement {
  const fileInput = useRef<HTMLInputElement>(null);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [dragOver, setDragOver] = useState(false);

  const tree = useMemo(() => buildTree(library.files), [library.files]);
  const matched = useMemo(
    () =>
      selection.mode === "glob" ? new Set(matchGlob(selection.pattern ?? "", library.paths)) : null,
    [selection, library.paths],
  );
  const selectedPaths = useMemo(
    () => new Set(resolveSelection(selection, library).map((f) => f.path)),
    [selection, library],
  );

  const offerSamples = useMemo(() => {
    const ids = sampleIds ?? SAMPLES.map((s) => s.id);
    const present = new Set(library.files.map((f) => f.path));
    return SAMPLES.filter((s) => ids.includes(s.id) && !present.has(s.filename));
  }, [sampleIds, library.files]);

  const isGlob = selection.mode === "glob";

  function setMode(mode: string) {
    if (!mode) return;
    if (mode === "glob") {
      onSelectionChange({ mode: "glob", paths: [], pattern: selection.pattern ?? "**/*" });
    } else {
      onSelectionChange({ mode: multiple ? "multi" : "single", paths: selection.paths });
    }
  }

  function chooseFile(path: string) {
    if (isGlob) return;
    if (selection.mode === "single" || !multiple) {
      onSelectionChange({ mode: "single", paths: [path] });
      return;
    }
    const set = new Set(selection.paths);
    if (set.has(path)) set.delete(path);
    else set.add(path);
    onSelectionChange({ mode: "multi", paths: [...set] });
  }

  function toggleCollapse(path: string) {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  async function onDrop(e: React.DragEvent) {
    e.preventDefault();
    setDragOver(false);
    if (e.dataTransfer.files.length) await library.upload(e.dataTransfer.files);
  }

  function renderDir(dir: TreeDir, depth: number): React.ReactNode {
    const isCollapsed = collapsed.has(dir.path);
    return (
      <React.Fragment key={dir.path}>
        <div
          className="flex items-center gap-2 px-2 py-1.5"
          style={{ paddingLeft: `${depth * 1.1 + 0.5}rem` }}
        >
          <button
            className="text-muted-foreground hover:text-foreground"
            onClick={() => toggleCollapse(dir.path)}
            aria-label={isCollapsed ? "Expand folder" : "Collapse folder"}
          >
            {isCollapsed ? (
              <ChevronRight className="size-3.5" />
            ) : (
              <ChevronDown className="size-3.5" />
            )}
          </button>
          <FolderOpen className="size-4 text-muted-foreground" aria-hidden />
          <span className="font-mono text-sm font-semibold">{dir.name}/</span>
          <span className="text-xs text-muted-foreground">{countFiles(dir)} files</span>
          <span className="ml-auto">
            <ConfirmDeleteButton mode="icon" onDelete={() => library.removeFolder(dir.path)} />
          </span>
        </div>
        {!isCollapsed && (
          <>
            {dir.dirs.map((d) => renderDir(d, depth + 1))}
            {dir.files.map((f) => renderFile(f, depth + 1))}
          </>
        )}
      </React.Fragment>
    );
  }

  function renderFile(f: LibFile, depth: number): React.ReactNode {
    const t = fileType(f.name);
    const isMatched = matched?.has(f.path) ?? false;
    const isSelected = selectedPaths.has(f.path);
    const dim = isGlob && !isMatched;
    return (
      <div
        key={f.path}
        className={cn(
          "group/file flex cursor-pointer items-center gap-2 px-2 py-1.5 text-sm transition-colors hover:bg-muted",
          isSelected && "bg-primary/10 hover:bg-primary/15",
          dim && "opacity-40",
        )}
        style={{ paddingLeft: `${depth * 1.1 + 0.5}rem` }}
        onClick={() => (onPreview ? onPreview(f) : chooseFile(f.path))}
      >
        {!isGlob && (
          <Checkbox
            checked={isSelected}
            onCheckedChange={() => chooseFile(f.path)}
            onClick={(e) => e.stopPropagation()}
            aria-label={`Select ${f.name}`}
          />
        )}
        {isGlob && (
          <span
            className={cn(
              "size-2 rounded-full border",
              isMatched
                ? "border-success bg-success ring-3 ring-success/25"
                : "border-muted-foreground/50",
            )}
            aria-hidden
          />
        )}
        <FileIcon filename={f.name} size={15} />
        <FileLabel name={f.name} title={f.path} className="min-w-0 flex-1" />
        <span className="flex shrink-0 items-center gap-2 whitespace-nowrap">
          <Badge
            variant={ORIGIN_VARIANT[f.origin]}
            className="text-[0.6rem] uppercase tracking-wide"
          >
            {f.origin}
          </Badge>
          <Badge variant="outline" className={cn("border-current/35 text-[0.65rem]", t.colorClass)}>
            {t.label}
          </Badge>
          <span className="w-12 text-right text-xs tabular-nums text-muted-foreground">
            {formatBytes(f.bytes.length)}
          </span>
          <span
            className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover/file:opacity-100 data-[selected=true]:opacity-100"
            data-selected={isSelected}
            onClick={(e) => e.stopPropagation()}
          >
            <Button
              variant="ghost"
              size="icon-xs"
              title="Download"
              aria-label={`Download ${f.name}`}
              onClick={() => downloadBytes(f.name, f.bytes)}
            >
              <Download />
            </Button>
            <ConfirmDeleteButton mode="icon" onDelete={() => library.remove(f.path)} />
          </span>
        </span>
      </div>
    );
  }

  const summary = selectionSummary(selection, library);

  return (
    <div
      className={cn(
        "kapi-reference flex flex-col gap-3 rounded-lg border border-transparent text-foreground transition-colors",
        dragOver && "border-dashed border-primary bg-primary/5",
        className,
      )}
      onDragOver={(e) => {
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={onDrop}
    >
      <div className="flex flex-wrap items-center gap-2">
        {multiple && (
          <ToggleGroup
            type="single"
            variant="outline"
            value={isGlob ? "glob" : "select"}
            onValueChange={setMode}
          >
            <ToggleGroupItem value="select" className="px-3 text-xs">
              Select
            </ToggleGroupItem>
            <ToggleGroupItem value="glob" className="gap-1 px-3 text-xs">
              <Regex className="size-3.5" /> Glob
            </ToggleGroupItem>
          </ToggleGroup>
        )}
        {isGlob && (
          <div className="min-w-56 flex-1">
            <GlobInput
              value={selection.pattern ?? ""}
              onChange={(v) => onSelectionChange({ mode: "glob", paths: [], pattern: v })}
              placeholder="e.g. **/*.json or *.{json,xliff}"
            />
          </div>
        )}
        <span className="flex-1" />
        <Button variant="outline" size="sm" onClick={() => fileInput.current?.click()}>
          <Upload /> Upload
        </Button>
        <input
          ref={fileInput}
          type="file"
          multiple
          hidden
          onChange={async (e) => {
            if (e.target.files?.length) await library.upload(e.target.files);
            e.target.value = "";
          }}
        />
      </div>

      {offerSamples.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">Add a sample:</span>
          {offerSamples.map((s) => (
            <Button
              key={s.id}
              variant="outline"
              size="xs"
              className="font-mono"
              title={s.blurb}
              onClick={() => library.addSample(s.id)}
            >
              <Plus />
              <FileIcon filename={s.filename} size={13} />
              {s.label}
            </Button>
          ))}
        </div>
      )}

      <div className="overflow-hidden rounded-lg border bg-card">
        {library.files.length === 0 ? (
          <EmptyState
            icon={<FolderOpen className="size-6" />}
            title="No files yet"
            description="Add a sample or drop your own file here."
          />
        ) : (
          <ScrollArea className="max-h-80">
            <div className="divide-y divide-border/60">
              {tree.dirs.map((d) => renderDir(d, 0))}
              {tree.files.map((f) => renderFile(f, 0))}
            </div>
          </ScrollArea>
        )}
      </div>

      <Separator />
      <div className="flex items-baseline gap-2 text-sm">
        <span className="font-mono text-foreground/80">{summary}</span>
        {isGlob && selection.pattern && (
          <span className="text-xs text-muted-foreground">
            matching across {library.files.length} files
          </span>
        )}
      </div>
    </div>
  );
}

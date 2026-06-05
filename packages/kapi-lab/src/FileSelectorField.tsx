import React, { useState } from "react";
import { ChevronDown, Files, Regex } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  cn,
} from "@neokapi/ui-primitives";
import { FileIcon } from "./fileTypes";
import { resolveSelection, selectionSummary } from "./fileLibrary";
import type { FileLibrary, FileSelection, LibFile } from "./fileLibrary";
import FileExplorer from "./FileExplorer";

export interface FileSelectorFieldProps {
  library: FileLibrary;
  selection: FileSelection;
  onSelectionChange: (sel: FileSelection) => void;
  multiple?: boolean;
  sampleIds?: string[];
  /** Field label, e.g. "Input file" / "Input files". */
  label?: string;
  onPreview?: (file: LibFile) => void;
  className?: string;
}

// FileSelectorField is the compact, always-visible control: it shows the
// current selection ("messages.json", "3 files", or a glob with its match
// count) and opens the full FileExplorer in a dialog when clicked. Explorers
// embed this so the file surface stays out of the way until you need it.
export default function FileSelectorField({
  library,
  selection,
  onSelectionChange,
  multiple = true,
  sampleIds,
  label = "Files",
  onPreview,
  className,
}: FileSelectorFieldProps): React.ReactElement {
  const [open, setOpen] = useState(false);

  const files = resolveSelection(selection, library);
  const summary = selectionSummary(selection, library);
  const single = files.length === 1 && selection.mode !== "glob";

  return (
    <div
      className={cn("kapi-reference flex flex-wrap items-center gap-2 text-foreground", className)}
    >
      <span className="text-sm text-muted-foreground">{label}</span>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <Button variant="outline" size="sm" className="min-w-48 max-w-md justify-start gap-2">
            {selection.mode === "glob" ? (
              <Regex className="size-4" />
            ) : single ? (
              <FileIcon filename={files[0].name} size={15} />
            ) : (
              <Files className="size-4" />
            )}
            <span className="flex-1 truncate text-left font-mono text-[0.8rem]">{summary}</span>
            <ChevronDown className="size-3.5 text-muted-foreground" />
          </Button>
        </DialogTrigger>
        <DialogContent className="kapi-reference max-w-3xl">
          <DialogHeader>
            <DialogTitle>Choose files</DialogTitle>
          </DialogHeader>
          <FileExplorer
            library={library}
            selection={selection}
            onSelectionChange={onSelectionChange}
            multiple={multiple}
            sampleIds={sampleIds}
            onPreview={onPreview}
          />
          <div className="flex justify-end">
            <Button onClick={() => setOpen(false)}>Done</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

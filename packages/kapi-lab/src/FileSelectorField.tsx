import React, { useEffect, useState } from "react";
import { ChevronDown, Files, Regex, X } from "lucide-react";
import {
  Button,
  Dialog,
  DialogClose,
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
  // Buffer the selection while the dialog is open so the embedded explorer edits
  // a draft; the parent (and its live re-run) only updates when the user commits
  // with "Done". Opening (re)syncs the draft to the committed selection.
  const [draft, setDraft] = useState<FileSelection>(selection);
  useEffect(() => {
    if (open) setDraft(selection);
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

  function commit() {
    onSelectionChange(draft);
    setOpen(false);
  }

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
        <DialogContent
          showCloseButton={false}
          onOpenAutoFocus={(e) => e.preventDefault()}
          className="kapi-reference w-[min(60rem,95vw)] max-w-none sm:max-w-none"
        >
          <DialogHeader className="flex-row items-center justify-between gap-4">
            <DialogTitle>{multiple ? "Choose files" : "Choose a file"}</DialogTitle>
            <DialogClose asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Close"
                className="text-muted-foreground hover:text-foreground"
              >
                <X />
              </Button>
            </DialogClose>
          </DialogHeader>
          <FileExplorer
            library={library}
            selection={draft}
            onSelectionChange={setDraft}
            multiple={multiple}
            sampleIds={sampleIds}
            onPreview={onPreview}
          />
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button onClick={commit}>Done</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

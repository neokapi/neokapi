import React, { useEffect, useMemo, useState } from "react";
import FileSelectorField from "./FileSelectorField";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";

export interface FileSourceValue {
  filename: string;
  label: string;
  /** Best-effort UTF-8 text, for display and as the source for text samples. */
  content: string;
  /** Raw bytes — what the engine reads, so binary formats (.docx, …) survive. */
  bytes?: Uint8Array;
}

interface FileSourceProps {
  value: FileSourceValue | null;
  onChange: (v: FileSourceValue) => void;
  /** Restrict the offered samples to these ids (default: all). */
  sampleIds?: string[];
  /** Field label (default "File"). */
  label?: string;
}

const dec = new TextDecoder();

// FileSource is the single-file picker the explorers embed. It is a thin compat
// wrapper over the shared FileSelectorField + file library, preserving the
// original value/onChange (FileSourceValue) API so explorers need no changes
// while getting the modern chooser (samples + uploads + outputs, type icons,
// download/delete) instead of the old inline chip list.
export default function FileSource({
  value,
  onChange,
  sampleIds,
  label = "File",
}: FileSourceProps): React.ReactElement {
  const library = useFileLibrary({ sampleIds });
  const [selection, setSelection] = useState<FileSelection>(() => ({
    mode: "single",
    paths: value ? [value.filename] : [],
  }));

  const selected = useMemo(() => resolveSelection(selection, library)[0], [selection, library]);

  // Emit the resolved file upward whenever it changes. onChange is the parent's
  // stable state setter; depend on the file identity, not the callback.
  useEffect(() => {
    if (!selected) return;
    onChange({
      filename: selected.path,
      label: selected.name,
      content: dec.decode(selected.bytes),
      bytes: selected.bytes,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected?.path, selected?.changedAt]);

  return (
    <FileSelectorField
      label={label}
      library={library}
      selection={selection}
      onSelectionChange={setSelection}
      multiple={false}
      sampleIds={sampleIds}
    />
  );
}

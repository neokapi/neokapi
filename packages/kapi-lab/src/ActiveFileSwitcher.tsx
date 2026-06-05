import React from "react";
import { Button, cn } from "@neokapi/ui-primitives";
import { FileIcon } from "./fileTypes";
import type { LibFile } from "./fileLibrary";

export interface ActiveFileSwitcherProps {
  /** The resolved selection (one chip per file). */
  files: LibFile[];
  /** The path currently being viewed/run. */
  activePath?: string;
  onChange: (path: string) => void;
  /** Leading label (default "Viewing"). */
  label?: string;
  className?: string;
}

// ActiveFileSwitcher lets a single-result explorer (Anatomy, Pipeline) work over
// a multi-file or glob selection: the chooser picks the working set, and this
// switches which file is currently inspected or run. It renders nothing for a
// selection of one — there is nothing to switch between.
export default function ActiveFileSwitcher({
  files,
  activePath,
  onChange,
  label = "Viewing",
  className,
}: ActiveFileSwitcherProps): React.ReactElement | null {
  if (files.length <= 1) return null;
  return (
    <div className={cn("flex flex-wrap items-center gap-1.5", className)}>
      <span className="text-xs text-muted-foreground">{label}</span>
      {files.map((f) => {
        const active = f.path === activePath;
        return (
          <Button
            key={f.path}
            size="xs"
            variant={active ? "default" : "outline"}
            className="gap-1 font-mono"
            onClick={() => onChange(f.path)}
            title={f.path}
          >
            <FileIcon filename={f.name} size={12} tinted={!active} />
            {f.name}
          </Button>
        );
      })}
    </div>
  );
}

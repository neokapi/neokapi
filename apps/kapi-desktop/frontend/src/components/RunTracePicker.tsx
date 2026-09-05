/**
 * The Run view's file picker: which of the last run's retained traces is
 * replaying. One file is named; several become a select. A trace the
 * recording budget cut short says how much of the file it holds.
 */

import { t } from "@neokapi/i18n-react/runtime";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import type { RunTraceFile } from "../types/api";
import { traceFileKey, traceFileLabel } from "../lib/runTraces";

interface RunTracePickerProps {
  /** The run's retained files, in completion order. */
  files: RunTraceFile[];
  selected: RunTraceFile;
  onSelect: (file: RunTraceFile) => void;
  /** The recording budget: a truncated trace holds this many parts. */
  maxParts: number;
}

export function RunTracePicker({ files, selected, onSelect, maxParts }: RunTracePickerProps) {
  return (
    <div
      className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground"
      data-testid="run-trace-picker"
    >
      {files.length > 1 ? (
        <Select
          value={traceFileKey(selected)}
          onValueChange={(key) => {
            const next = files.find((f) => traceFileKey(f) === key);
            if (next) onSelect(next);
          }}
        >
          <SelectTrigger size="sm" aria-label={t("Run file")} data-testid="run-trace-file">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {files.map((f) => (
              <SelectItem key={traceFileKey(f)} value={traceFileKey(f)}>
                {traceFileLabel(f)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <span className="truncate" title={selected.file_path} data-testid="run-trace-file">
          {traceFileLabel(selected)}
        </span>
      )}
      {selected.truncated && (
        <span data-testid="run-trace-truncated">
          {t("First {count} parts", { count: maxParts })}
        </span>
      )}
    </div>
  );
}

import { useQuery } from "@tanstack/react-query";
import { FileText, Loader2, FileWarning } from "lucide-react";
import { ListCapRow, SimpleTooltip } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";

// An archive can hold thousands of inner files. The entry list is scroll-contained
// (bounded height) and its rows are capped with an honest ListCapRow, so browsing
// a large archive never mounts a giant nested DOM under a table row.
const ARCHIVE_ENTRY_CAP = 400;

export interface ArchiveEntry {
  name: string;
  format: string;
  size: number;
}

const ARCHIVE_EXTS = [".zip", ".tar", ".tgz", ".tar.gz"];

/** isArchivePath reports whether a path names a browsable archive container. */
export function isArchivePath(name: string): boolean {
  const lower = name.toLowerCase();
  return ARCHIVE_EXTS.some((ext) => lower.endsWith(ext));
}

function humanSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export interface ArchiveEntriesProps {
  /** Absolute path of the archive container. */
  archivePath: string;
  /** Called with the inner entry path when the user picks an entry to preview. */
  onSelect: (entry: string) => void;
  /** Pre-loaded entries for Storybook/tests, skipping the backend call. */
  entries?: ArchiveEntry[];
}

// ArchiveEntries lists the inner files of an archive (lazily, via
// ListArchiveEntries) as a nested group under the archive's row. Each recognised
// entry is selectable and opens a per-entry preview (InspectArchiveEntry);
// entries kapi has no reader for are shown but disabled.
export function ArchiveEntries({ archivePath, onSelect, entries: preset }: ArchiveEntriesProps) {
  const query = useQuery({
    queryKey: qk.archiveEntries(archivePath),
    queryFn: () => api.listArchiveEntries(archivePath),
    enabled: !preset,
  });

  const entries = preset ?? query.data ?? null;
  const loading = !preset && query.isLoading;
  const error = query.error
    ? query.error instanceof Error
      ? query.error.message
      : String(query.error)
    : // A null result (no Wails runtime) is not an exception but still unavailable.
      !preset && query.isSuccess && query.data === null
      ? "Archive listing is unavailable in this environment."
      : null;

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-2 pl-8 text-xs text-muted-foreground">
        <Loader2 className="size-3 animate-spin" />
        Listing entries…
      </div>
    );
  }
  if (error) {
    return (
      <div className="flex items-center gap-2 py-2 pl-8 text-xs text-destructive">
        <FileWarning className="size-3" />
        {error}
      </div>
    );
  }
  if (!entries || entries.length === 0) {
    return <div className="py-2 pl-8 text-xs text-muted-foreground">No entries.</div>;
  }

  const shown = entries.slice(0, ARCHIVE_ENTRY_CAP);
  return (
    <div className="border-l border-border/60 pl-3">
      <ul
        className="max-h-72 overflow-y-auto"
        aria-label="Archive entries"
        data-slot="archive-entries"
      >
        {shown.map((e) => (
          <li key={e.name}>
            <SimpleTooltip
              content={e.format ? `Preview ${e.name}` : "No reader for this file type"}
            >
              <span className="block w-full">
                <button
                  type="button"
                  disabled={!e.format}
                  onClick={e.format ? () => onSelect(e.name) : undefined}
                  className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-accent disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent"
                >
                  <FileText className="size-3 shrink-0 text-muted-foreground" />
                  <span className="truncate font-mono" translate="no">
                    {e.name}
                  </span>
                  <span className="ml-auto shrink-0 text-muted-foreground">
                    {humanSize(e.size)}
                  </span>
                </button>
              </span>
            </SimpleTooltip>
          </li>
        ))}
      </ul>
      <ListCapRow
        shown={shown.length}
        total={entries.length}
        noun="entries"
        hint="Extract the archive to browse all of its entries."
        className="border-t-0"
      />
    </div>
  );
}

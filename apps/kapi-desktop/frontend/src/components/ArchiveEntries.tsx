import { useEffect, useState } from "react";
import { FileText, Loader2, FileWarning } from "lucide-react";
import { api } from "../hooks/useApi";

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
  const [entries, setEntries] = useState<ArchiveEntry[] | null>(preset ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (preset) {
      setEntries(preset);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .listArchiveEntries(archivePath)
      .then((list) => {
        if (cancelled) return;
        if (list === null) {
          setError("Archive listing is unavailable in this environment.");
          return;
        }
        setEntries(list);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [archivePath, preset]);

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

  return (
    <ul className="border-l border-border/60 pl-3" aria-label="Archive entries">
      {entries.map((e) => (
        <li key={e.name}>
          <button
            type="button"
            disabled={!e.format}
            onClick={e.format ? () => onSelect(e.name) : undefined}
            className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-accent disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent"
            title={e.format ? `Preview ${e.name}` : "No reader for this file type"}
          >
            <FileText className="size-3 shrink-0 text-muted-foreground" />
            <span className="truncate font-mono" translate="no">
              {e.name}
            </span>
            <span className="ml-auto shrink-0 text-muted-foreground">{humanSize(e.size)}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

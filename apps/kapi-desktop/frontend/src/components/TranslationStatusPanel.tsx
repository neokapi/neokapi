import { useEffect, useState } from "react";
import { Card, CardContent } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";

export interface CollectionStatus {
  name: string;
  archive: string;
  archiveExists: boolean;
  blockCount: number;
  coverage: Record<string, number>;
  targetLanguages: string[];
}

export interface ProjectStatus {
  projectPath: string;
  projectName: string;
  collections: CollectionStatus[];
}

export interface TranslationStatusPanelProps {
  tabID: string;
  /** Pre-loaded status for Storybook — skips the Wails call. */
  status?: ProjectStatus;
}

/**
 * Renders the translation-state panel for a .kapi project: one
 * section per ContentCollection, showing archive path and per-locale
 * coverage bars. Data mirrors `kapi status` output.
 */
export function TranslationStatusPanel({ tabID, status: propStatus }: TranslationStatusPanelProps) {
  const [status, setStatus] = useState<ProjectStatus | null>(propStatus ?? null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (propStatus) return;
    let cancelled = false;
    api
      .getProjectStatus(tabID)
      .then((s) => {
        if (!cancelled) setStatus(s);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [tabID, propStatus]);

  if (error) {
    return (
      <div className="p-4 text-sm text-destructive" data-slot="translation-status-error">
        {error}
      </div>
    );
  }
  if (!status) {
    return (
      <div className="p-4 text-sm text-muted-foreground" data-slot="translation-status-loading">
        Loading translation status…
      </div>
    );
  }

  return (
    <div className="space-y-3" data-slot="translation-status-panel">
      {status.collections.length === 0 && (
        <div className="p-4 text-sm text-muted-foreground">
          No content collections defined in this project.
        </div>
      )}
      {status.collections.map((collection) => (
        <CollectionCard key={`${collection.name}|${collection.archive}`} collection={collection} />
      ))}
    </div>
  );
}

function CollectionCard({ collection }: { collection: CollectionStatus }) {
  if (!collection.archive) {
    return (
      <Card>
        <CardContent className="space-y-1 p-4">
          <h3 className="text-sm font-medium">{collection.name}</h3>
          <p className="text-xs text-muted-foreground">
            No archive — this collection uses a file-based flow. Nothing to report.
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <header className="flex items-baseline justify-between gap-2">
          <h3 className="text-sm font-medium">{collection.name}</h3>
          <code className="text-xs text-muted-foreground">{collection.archive}</code>
        </header>
        {!collection.archiveExists ? (
          <p className="text-xs text-destructive">
            Archive missing — run <code>kapi-react extract</code> to create it.
          </p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">{collection.blockCount} blocks</p>
            <LocaleCoverage collection={collection} />
          </>
        )}
      </CardContent>
    </Card>
  );
}

function LocaleCoverage({ collection }: { collection: CollectionStatus }) {
  const locales = Array.from(
    new Set([...collection.targetLanguages, ...Object.keys(collection.coverage)]),
  ).sort();

  if (locales.length === 0) {
    return <p className="text-xs text-muted-foreground">No target languages declared.</p>;
  }

  return (
    <ul className="space-y-1.5" data-slot="locale-coverage">
      {locales.map((loc) => {
        const translated = collection.coverage[loc] ?? 0;
        const pct = collection.blockCount > 0 ? (translated / collection.blockCount) * 100 : 0;
        const state = translated === 0 ? "empty" : pct >= 100 ? "complete" : "partial";
        return (
          <li key={loc} className="flex items-center gap-2 text-xs" data-locale={loc}>
            <span className="w-10 font-mono uppercase">{loc}</span>
            <div className="flex-1 overflow-hidden rounded-full bg-muted">
              <div
                className={
                  state === "complete"
                    ? "h-1.5 bg-primary"
                    : state === "partial"
                      ? "h-1.5 bg-primary/60"
                      : "h-1.5 bg-muted-foreground/20"
                }
                style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
                data-state={state}
              />
            </div>
            <span className="w-24 shrink-0 text-right tabular-nums text-muted-foreground">
              {translated === 0
                ? "not translated"
                : translated === collection.blockCount
                  ? `${translated}/${collection.blockCount} · complete`
                  : `${translated}/${collection.blockCount}`}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

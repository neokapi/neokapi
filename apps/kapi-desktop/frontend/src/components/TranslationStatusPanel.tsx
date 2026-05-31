import { useCallback, useEffect, useState } from "react";
import { Button, Card, CardContent } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import { Loader2, RefreshCw } from "lucide-react";
import { api } from "../hooks/useApi";

export interface CollectionStatus {
  name: string;
  blockCount?: number;
  coverage?: Record<string, number>;
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
 * Renders the translation-state panel for a kapi project: one
 * section per ContentCollection, with target locales and (once the
 * blockstore session wiring lands in kapi-desktop) per-locale
 * coverage bars. Currently driven by the recipe's declared target
 * languages only — coverage data becomes available after the
 * blockstore integration step.
 */
export function TranslationStatusPanel({ tabID, status: propStatus }: TranslationStatusPanelProps) {
  const [status, setStatus] = useState<ProjectStatus | null>(propStatus ?? null);
  const [error, setError] = useState<string | null>(null);
  const [extracting, setExtracting] = useState(false);
  const [extractLog, setExtractLog] = useState<string | null>(null);

  const refreshStatus = useCallback(() => {
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

  useEffect(() => {
    refreshStatus();
  }, [refreshStatus]);

  const runExtract = useCallback(async () => {
    setExtracting(true);
    setError(null);
    try {
      const result = await api.runExtract(tabID);
      if (result) setExtractLog(result.log);
      refreshStatus();
    } catch (e) {
      setError(String(e));
    } finally {
      setExtracting(false);
    }
  }, [tabID, refreshStatus]);

  if (error && !status) {
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

  const hasCollections = status.collections.length > 0;

  return (
    <div className="space-y-3" data-slot="translation-status-panel">
      <div className="flex items-center justify-between">
        <div className="text-xs text-muted-foreground">
          {extractLog && !extracting && "Last extract output available below."}
        </div>
        {hasCollections && (
          <Button
            variant="outline"
            size="xs"
            onClick={() => void runExtract()}
            disabled={extracting}
            data-slot="translation-status-reextract"
            aria-label="Re-extract translatable content"
          >
            {extracting ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
            {extracting ? t("Extracting…") : t("Re-extract")}
          </Button>
        )}
      </div>
      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
          {error}
        </div>
      )}
      {status.collections.length === 0 && (
        <div className="p-4 text-sm text-muted-foreground">
          No content collections defined in this project.
        </div>
      )}
      {status.collections.map((collection) => (
        <CollectionCard key={collection.name} collection={collection} />
      ))}
      {extractLog && (
        <pre
          className="max-h-48 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-[11px] text-muted-foreground"
          data-slot="translation-status-log"
        >
          {extractLog.trim()}
        </pre>
      )}
    </div>
  );
}

function CollectionCard({ collection }: { collection: CollectionStatus }) {
  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <header className="flex items-baseline justify-between gap-2">
          <h3 className="text-sm font-medium">{collection.name}</h3>
          {typeof collection.blockCount === "number" && (
            <span className="text-xs text-muted-foreground">{collection.blockCount} blocks</span>
          )}
        </header>
        <LocaleCoverage collection={collection} />
      </CardContent>
    </Card>
  );
}

function LocaleCoverage({ collection }: { collection: CollectionStatus }) {
  const coverage = collection.coverage ?? {};
  const locales = Array.from(
    new Set([...collection.targetLanguages, ...Object.keys(coverage)]),
  ).sort();

  if (locales.length === 0) {
    return <p className="text-xs text-muted-foreground">No target languages declared.</p>;
  }

  return (
    <ul className="space-y-1.5" data-slot="locale-coverage">
      {locales.map((loc) => {
        const translated = coverage[loc] ?? 0;
        const total = collection.blockCount ?? 0;
        const pct = total > 0 ? (translated / total) * 100 : 0;
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
              {total === 0
                ? t("pending")
                : translated === 0
                  ? t("not translated")
                  : translated === total
                    ? t("{translated}/{total} · complete", { translated, total })
                    : `${translated}/${total}`}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

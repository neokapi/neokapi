import { useCallback, useEffect, useState } from "react";
import { Button, Card, CardContent } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import { Check, CheckCircle2, ClipboardCheck, Loader2, PlayCircle, RefreshCw } from "lucide-react";
import { api } from "../hooks/useApi";
import type {
  ConvergenceReport,
  GateShortfall,
  LocaleCoverage,
  ReviewItem,
  SourceCoverage,
} from "../types/api";

// The lifecycle ladders, lowest → highest. The Pct map carries "at least this
// rung" percentages, so a segmented bar reads as deepening progress.
const TARGET_LADDER = ["draft", "translated", "reviewed", "signed-off"];
const SOURCE_LADDER = ["authored", "checked", "approved"];

export interface ConvergencePanelProps {
  tabID: string;
  /** Pre-loaded report for Storybook/tests — skips the Wails call. */
  report?: ConvergenceReport;
  /** Override the approve action (tests/Storybook); defaults to the Wails call. */
  onApprove?: (item: ReviewItem) => Promise<void>;
}

/**
 * The convergence surface for a kapi project: per-locale coverage along the
 * target lifecycle ladder with each scope's ship-gate standing, source-authoring
 * readiness, and the review queue (translated units awaiting a human). A "Bring
 * up to date" action runs the project's default flow over every locale. It shows
 * the same derived state `kapi status` / `kapi verify` report — the desktop
 * embodiment of the convergence model.
 */
export function ConvergencePanel({ tabID, report: propReport, onApprove }: ConvergencePanelProps) {
  const [report, setReport] = useState<ConvergenceReport | null>(propReport ?? null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [approving, setApproving] = useState<Set<string>>(new Set());

  const refresh = useCallback(() => {
    if (propReport) return;
    let cancelled = false;
    api
      .getConvergence(tabID)
      .then((r) => {
        if (!cancelled) setReport(r);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [tabID, propReport]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const bringUpToDate = useCallback(async () => {
    setError(null);
    setRunning(true);
    try {
      await api.bringUpToDate(tabID);
      // Poll the shared run state until the launched flow reaches a terminal
      // state, then re-derive coverage.
      for (let i = 0; i < 600; i++) {
        const state = await api.getRunState();
        if (state === "complete" || state === "error" || state === "canceled" || state == null) {
          break;
        }
        await new Promise((r) => setTimeout(r, 500));
      }
      refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setRunning(false);
    }
  }, [tabID, refresh]);

  const approve = useCallback(
    async (item: ReviewItem) => {
      const id = `${item.locale}:${item.file}:${item.key}`;
      setApproving((s) => new Set(s).add(id));
      setError(null);
      try {
        if (onApprove) {
          await onApprove(item);
        } else {
          await api.approveReviewItem(tabID, item.locale, item.file, item.key);
        }
        refresh();
      } catch (e) {
        setError(String(e));
      } finally {
        setApproving((s) => {
          const next = new Set(s);
          next.delete(id);
          return next;
        });
      }
    },
    [tabID, onApprove, refresh],
  );

  if (error && !report) {
    return (
      <div className="p-4 text-sm text-destructive" data-slot="convergence-error">
        {error}
      </div>
    );
  }
  if (!report) {
    return (
      <div className="p-4 text-sm text-muted-foreground" data-slot="convergence-loading">
        {t("Loading convergence status…")}
      </div>
    );
  }

  const locales = report.locales ?? [];
  const review = report.review ?? [];

  return (
    <div className="space-y-3" data-slot="convergence-panel">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">{t("Convergence")}</h3>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="xs"
            onClick={() => refresh()}
            disabled={running}
            aria-label={t("Refresh convergence status")}
            data-slot="convergence-refresh"
          >
            <RefreshCw size={12} />
          </Button>
          <Button
            size="xs"
            onClick={() => void bringUpToDate()}
            disabled={running}
            data-slot="convergence-bring-up-to-date"
          >
            {running ? <Loader2 size={12} className="animate-spin" /> : <PlayCircle size={12} />}
            {running ? t("Bringing up to date…") : t("Bring up to date")}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
          {error}
        </div>
      )}

      {report.source && <SourceReadinessRow source={report.source} />}

      {locales.length === 0 ? (
        <p className="p-3 text-xs text-muted-foreground">
          {t("No localized content tracked yet.")}
        </p>
      ) : (
        <Card>
          <CardContent className="space-y-2.5 p-4" data-slot="convergence-coverage">
            {locales.map((lc) => (
              <CoverageRow key={`${lc.locale}/${lc.collection ?? ""}`} cov={lc} />
            ))}
          </CardContent>
        </Card>
      )}

      <ReviewQueue items={review} approving={approving} onApprove={approve} />
    </div>
  );
}

function scopeLabel(cov: LocaleCoverage): string {
  return cov.collection ? `${cov.locale}/${cov.collection}` : cov.locale;
}

function CoverageRow({ cov }: { cov: LocaleCoverage }) {
  return (
    <div className="flex items-center gap-3 text-xs" data-locale={cov.locale}>
      <span className="w-24 shrink-0 truncate font-mono uppercase" title={scopeLabel(cov)}>
        {scopeLabel(cov)}
      </span>
      <LadderBar pct={cov.pct} ladder={TARGET_LADDER} />
      <ShipStanding cov={cov} />
    </div>
  );
}

// LadderBar renders a single track segmented by the "at least" percentages: the
// darkest band is the highest rung reached, fading down to the lowest, with the
// remainder left empty.
function LadderBar({ pct, ladder }: { pct: Record<string, number>; ladder: string[] }) {
  // Tones from highest rung (darkest) to lowest (lightest).
  const tones = ["bg-primary", "bg-primary/70", "bg-primary/45", "bg-primary/25"];
  const highToLow = [...ladder].reverse();
  const segments: { width: number; tone: string; state: string }[] = [];
  let covered = 0; // percent already drawn (higher rungs)
  highToLow.forEach((state, i) => {
    const at = Math.max(0, Math.min(100, pct[state] ?? 0));
    const width = Math.max(0, at - covered);
    if (width > 0) {
      segments.push({ width, tone: tones[Math.min(i, tones.length - 1)], state });
      covered = at;
    }
  });
  return (
    <div
      className="flex h-2 flex-1 overflow-hidden rounded-full bg-muted"
      role="img"
      aria-label={ladder.map((s) => `${s} ${pct[s] ?? 0}%`).join(", ")}
    >
      {segments.map((seg, i) => (
        <div
          key={i}
          className={`h-2 ${seg.tone}`}
          style={{ width: `${seg.width}%` }}
          data-state={seg.state}
        />
      ))}
    </div>
  );
}

function ShipStanding({ cov }: { cov: LocaleCoverage }) {
  if (!cov.gated) {
    return <span className="w-28 shrink-0 text-right text-muted-foreground">—</span>;
  }
  if (cov.shippable) {
    return (
      <span className="flex w-28 shrink-0 items-center justify-end gap-1 text-right text-primary">
        <CheckCircle2 size={12} /> {t("shippable")}
      </span>
    );
  }
  return (
    <span
      className="w-28 shrink-0 text-right tabular-nums text-amber-600 dark:text-amber-500"
      title={(cov.pending ?? []).map(shortfallText).join(", ")}
    >
      {t("pending")}
    </span>
  );
}

function shortfallText(s: GateShortfall): string {
  return `${s.state} ${Math.round(s.actual)}%<${s.required}%`;
}

function SourceReadinessRow({ source }: { source: SourceCoverage }) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-3 text-xs" data-slot="convergence-source">
        <span className="w-24 shrink-0 font-medium">{t("source")}</span>
        <LadderBar pct={source.pct} ladder={SOURCE_LADDER} />
        {source.gated ? (
          source.shippable ? (
            <span className="flex w-28 shrink-0 items-center justify-end gap-1 text-right text-primary">
              <CheckCircle2 size={12} /> {t("ready")}
            </span>
          ) : (
            <span
              className="w-28 shrink-0 text-right text-amber-600 dark:text-amber-500"
              title={(source.pending ?? []).map(shortfallText).join(", ")}
            >
              {t("pending")}
            </span>
          )
        ) : (
          <span className="w-28 shrink-0 text-right text-muted-foreground">—</span>
        )}
      </CardContent>
    </Card>
  );
}

function ReviewQueue({
  items,
  approving,
  onApprove,
}: {
  items: ReviewItem[];
  approving: Set<string>;
  onApprove: (item: ReviewItem) => void;
}) {
  if (items.length === 0) {
    return (
      <div
        className="flex items-center gap-2 p-2 text-xs text-muted-foreground"
        data-slot="convergence-review-empty"
      >
        <CheckCircle2 size={12} className="text-primary" />
        {t("Review queue empty — every translated unit is reviewed.")}
      </div>
    );
  }
  return (
    <Card>
      <CardContent className="space-y-2 p-4" data-slot="convergence-review">
        <header className="flex items-center gap-2 text-sm font-medium">
          <ClipboardCheck size={14} />
          {t("{count} awaiting review", { count: items.length })}
        </header>
        <ul className="space-y-1.5">
          {items.slice(0, 50).map((it, i) => {
            const id = `${it.locale}:${it.file}:${it.key}`;
            const busy = approving.has(id);
            return (
              <li
                key={`${it.file}:${it.key}:${i}`}
                className="flex items-start gap-2 text-xs"
                data-locale={it.locale}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono uppercase text-muted-foreground">{it.locale}</span>
                    <span className="truncate text-muted-foreground" title={`${it.file}:${it.key}`}>
                      {it.file}:{it.key}
                    </span>
                  </div>
                  <div className="truncate pl-1" title={it.source}>
                    {it.source}
                  </div>
                  {it.target && (
                    <div className="truncate pl-1 text-muted-foreground" title={it.target}>
                      → {it.target}
                    </div>
                  )}
                </div>
                <Button
                  variant="outline"
                  size="xs"
                  className="shrink-0"
                  disabled={busy}
                  onClick={() => onApprove(it)}
                  data-slot="convergence-review-approve"
                  aria-label={t("Approve this translation")}
                >
                  {busy ? <Loader2 size={12} className="animate-spin" /> : <Check size={12} />}
                  {t("Approve")}
                </Button>
              </li>
            );
          })}
        </ul>
        {items.length > 50 && (
          <p className="text-xs text-muted-foreground">
            {t("…and {count} more", { count: items.length - 50 })}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

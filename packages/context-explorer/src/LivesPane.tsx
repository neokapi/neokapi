// "What lives here" — the content at the point (Apache-2.0).
//
// Coverage and ship states are derived, never stored: a decision cites the
// source hash it blessed, and a basis that no longer matches the current hash is
// stale on every stream at once. The pane shows the derivation's verdict and
// flags the stale rows rather than presenting a count that has quietly rotted.

import { Badge, Progress, Separator, cn } from "@neokapi/ui-primitives";
import { EmptyHint, LocalePill, useResource } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { FileText, Files, TriangleAlert } from "lucide-react";
import { useScope } from "./ScopeProvider";
import { NoteStrip, Pane, PaneError, PaneSkeleton, ReachNote } from "./atoms";
import type { ContentAnswer, LocaleCoverage } from "./types";

export interface LivesPaneProps {
  /** Cap on the items listed. */
  limit?: number;
  /**
   * Called when a collection row is chosen, so the ladder can descend. Carries
   * the collection's ID, not its label: the tuple addresses a collection the
   * same way the filter bar's options do, and a workspace's ids are not its
   * names.
   */
  onOpenCollection?: (collectionID: string) => void;
  /** Called when a content row is chosen. */
  onOpenItem?: (item: { id: string; document?: string }) => void;
  className?: string;
}

const SHIP_TONE: Record<string, string> = {
  governed: "border-emerald-500/40 text-emerald-700 dark:text-emerald-500",
  "ai-shippable": "border-sky-500/40 text-sky-700 dark:text-sky-400",
  pending: "border-amber-500/40 text-amber-700 dark:text-amber-500",
};

function ShipBadge({ state }: { state?: string }) {
  if (!state) return null;
  return (
    <Badge
      variant="outline"
      className={cn("font-normal", SHIP_TONE[state])}
      data-ship-state={state}
    >
      {state}
    </Badge>
  );
}

function CoverageRow({ row }: { row: LocaleCoverage }) {
  const pct = row.units > 0 ? Math.round((row.covered / row.units) * 100) : 0;
  return (
    <li
      className="flex items-center gap-3 py-1.5"
      data-testid="lives-coverage"
      data-locale={row.locale}
    >
      <LocalePill locale={row.locale} />
      <Progress value={pct} className="h-1.5 w-24" />
      <span className="w-10 text-right text-xs tabular-nums text-muted-foreground">{pct}%</span>
      <ShipBadge state={row.ship_state} />
      {row.stale ? (
        <span className="flex items-center gap-1 text-xs text-amber-700 dark:text-amber-500">
          <TriangleAlert className="size-3.5" />
          {t("{count} stale", { count: row.stale })}
        </span>
      ) : null}
    </li>
  );
}

export function LivesPane({ limit, onOpenCollection, onOpenItem, className }: LivesPaneProps) {
  const { scope, source, capabilities } = useScope();

  const { data, loading, error } = useResource<ContentAnswer | null>(
    () => (source.lives ? source.lives({ scope, limit }) : null),
    [
      scope.workspace,
      scope.project,
      scope.stream,
      scope.collection,
      scope.path,
      scope.locale,
      limit,
    ],
  );

  if (!capabilities.content) {
    return (
      <Pane
        title={t("What lives here")}
        icon={<Files />}
        className={className}
        data-testid="lives-pane"
      >
        <ReachNote>{t("This surface does not read content at a point.")}</ReachNote>
      </Pane>
    );
  }

  return (
    <Pane
      title={t("What lives here")}
      icon={<Files />}
      reach={data ? data.scope : undefined}
      className={className}
      data-testid="lives-pane"
    >
      {error && <PaneError error={error} />}
      {!error && loading && <PaneSkeleton />}
      {!error && !loading && data && (
        <div className="space-y-4 text-sm">
          <NoteStrip notes={data.notes} />

          {data.coverage.length > 0 && (
            <section>
              <h4 className="mb-1 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                {t("Coverage")}
              </h4>
              <ul className="divide-y">
                {data.coverage.map((row) => (
                  <CoverageRow key={row.locale} row={row} />
                ))}
              </ul>
            </section>
          )}

          {data.collections.length > 0 && (
            <>
              {data.coverage.length > 0 && <Separator />}
              <section>
                <h4 className="mb-1 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                  {t("Collections")}
                </h4>
                <ul className="divide-y">
                  {data.collections.map((c) => (
                    <li key={`${c.project ?? ""}:${c.id}`} data-testid="lives-collection">
                      <button
                        type="button"
                        className="flex w-full items-center gap-2 py-1.5 text-left hover:text-primary"
                        onClick={() => onOpenCollection?.(c.id)}
                      >
                        <span className="font-medium">{c.name}</span>
                        {c.project && (
                          <span className="text-xs text-muted-foreground">{c.project}</span>
                        )}
                        {c.point && (
                          <Badge variant="outline" className="font-mono text-[11px] font-normal">
                            {c.point}
                          </Badge>
                        )}
                        {c.items !== undefined && (
                          <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                            {c.items}
                          </span>
                        )}
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}

          <Separator />
          <section>
            <h4 className="mb-1 flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <FileText className="size-3.5" />
              {t("Content")}
              {data.items_total !== undefined && data.items_total > data.items.length && (
                <span className="font-normal normal-case">
                  {t("{shown} of {total}", { shown: data.items.length, total: data.items_total })}
                </span>
              )}
            </h4>
            {data.items.length > 0 ? (
              <ul className="divide-y">
                {data.items.map((item) => (
                  <li key={item.id} data-testid="lives-item" data-stale={item.stale}>
                    <button
                      type="button"
                      className="flex w-full items-start gap-2 py-1.5 text-left hover:text-primary"
                      onClick={() => onOpenItem?.(item)}
                    >
                      {item.locale && <LocalePill locale={item.locale} />}
                      <span className="min-w-0 flex-1 truncate">{item.text ?? item.id}</span>
                      {item.stale && (
                        <TriangleAlert className="mt-0.5 size-3.5 shrink-0 text-amber-600 dark:text-amber-500" />
                      )}
                      <ShipBadge state={item.ship_state} />
                    </button>
                    {item.document && (
                      <p className="pb-1.5 font-mono text-[11px] text-muted-foreground">
                        {item.document}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyHint
                title={t("No content at this point")}
                description={t("Narrow or widen the scope to find where content sits.")}
              />
            )}
          </section>
        </div>
      )}
    </Pane>
  );
}

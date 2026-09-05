// "How it relates" — occurrence, blessing, membership (Apache-2.0).
//
// The pane states its reach in its own header. A project-reach answer inside a
// workspace is a smaller answer, not a wrong one, and saying so is the whole
// contract: "which projects use this concept" answering from one project would
// read as "only this project uses it", which is a lie the graph never told.

import { Badge, Separator, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { EmptyHint, LocalePill, useResource } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { GitBranch, Quote, Stamp } from "lucide-react";
import { useScope } from "./ScopeProvider";
import { NoteStrip, Pane, PaneError, PaneSkeleton, ReachNote } from "./atoms";
import type { RelationSubject, RelationsAnswer } from "./types";

export interface RelatesPaneProps {
  /** What the question is about. The pane is idle until a subject is selected. */
  subject?: RelationSubject;
  limit?: number;
  /** Called when an occurrence row names a place the ladder can move to. */
  onOpenOccurrence?: (place: { project?: string; collection?: string; document?: string }) => void;
  className?: string;
}

export function RelatesPane({ subject, limit, onOpenOccurrence, className }: RelatesPaneProps) {
  const { scope, source, capabilities } = useScope();

  const { data, loading, error } = useResource<RelationsAnswer | null>(
    () => (source.relates && subject ? source.relates(subject, { scope, limit }) : null),
    [
      subject?.kind,
      subject?.id,
      scope.workspace,
      scope.project,
      scope.stream,
      scope.collection,
      limit,
    ],
  );

  if (capabilities.relations === "none") {
    return (
      <Pane
        title={t("How it relates")}
        icon={<GitBranch />}
        className={className}
        data-testid="relates-pane"
      >
        <ReachNote>{t("This surface holds no relations for a selection.")}</ReachNote>
      </Pane>
    );
  }

  const reach =
    (data?.reach ?? capabilities.relations) === "workspace"
      ? t("across the workspace")
      : t("this project only");

  return (
    <Pane
      title={t("How it relates")}
      icon={<GitBranch />}
      reach={reach}
      className={className}
      data-testid="relates-pane"
      data-reach={data?.reach ?? capabilities.relations}
    >
      {!subject && (
        <EmptyHint
          title={t("Nothing selected")}
          description={t("Choose a term or a content item to see where else it appears.")}
        />
      )}
      {subject && error && <PaneError error={error} />}
      {subject && !error && loading && <PaneSkeleton />}
      {subject && !error && !loading && data && (
        <div className="space-y-4 text-sm">
          <NoteStrip notes={data.notes} />

          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary" className="font-normal">
              {data.subject.kind}
            </Badge>
            <span className="font-medium">{data.subject.label ?? data.subject.id}</span>
          </div>

          {capabilities.relations === "project" && (
            <ReachNote>
              {t(
                "This answer covers the project in scope. A workspace answers the same question across every project that pushed.",
              )}
            </ReachNote>
          )}

          {data.projects.length > 0 && (
            <section data-testid="relates-projects">
              <h4 className="mb-1 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                {t("Projects")}
              </h4>
              <ul className="divide-y">
                {data.projects.map((p) => (
                  <li
                    key={`${p.project}:${p.stream ?? ""}`}
                    className="flex flex-wrap items-center gap-2 py-1.5"
                    data-testid="relates-project"
                    data-discouraged={p.discouraged}
                  >
                    <span className="font-medium">{p.project_name ?? p.project}</span>
                    {p.stream && (
                      <code className="font-mono text-[11px] text-muted-foreground">
                        @{p.stream}
                      </code>
                    )}
                    {/* How the concept stands HERE. The status is the project's
                        own usage, not the workspace's opinion of the word. */}
                    {p.status && (
                      <Badge
                        variant={p.discouraged ? "destructive" : "outline"}
                        className="font-normal"
                        data-testid="relates-project-status"
                      >
                        {p.discouraged && p.replacement
                          ? t("{status} · say {replacement}", {
                              status: p.status,
                              replacement: p.replacement,
                            })
                          : p.status}
                      </Badge>
                    )}
                    {p.collections && p.collections.length > 0 && (
                      <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                        {p.collections.join(", ")}
                      </span>
                    )}
                    {p.occurrences !== undefined && (
                      // Read from the graph's edges, so the number stands as of
                      // the last extraction on this surface and the platform's.
                      <SimpleTooltip content={t("as of the last extraction")}>
                        <span
                          className="ml-auto text-xs tabular-nums text-muted-foreground"
                          data-testid="relates-project-uses"
                        >
                          {t("{count} uses", { count: p.occurrences })}
                        </span>
                      </SimpleTooltip>
                    )}
                    {p.terms && p.terms.length > 0 && (
                      <span className="w-full text-xs text-muted-foreground">
                        {p.terms
                          .map((term) =>
                            term.discouraged
                              ? t("{term} ({status})", {
                                  term: term.term,
                                  status: term.status ?? "",
                                })
                              : term.term,
                          )
                          .join(", ")}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}

          <section data-testid="relates-occurrences">
            <h4 className="mb-1 flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <Quote className="size-3.5" />
              {t("Occurrence")}
              {data.occurrences_total !== undefined &&
                data.occurrences_total > data.occurrences.length && (
                  <span className="font-normal normal-case">
                    {t("{shown} of {total}", {
                      shown: data.occurrences.length,
                      total: data.occurrences_total,
                    })}
                  </span>
                )}
            </h4>
            {data.occurrences.length > 0 ? (
              <ul className="divide-y">
                {data.occurrences.map((o, i) => (
                  <li key={`${o.block_id ?? o.document ?? ""}:${i}`}>
                    <button
                      type="button"
                      className="flex w-full flex-col items-start gap-0.5 py-1.5 text-left hover:text-primary"
                      data-testid="relates-occurrence"
                      onClick={() =>
                        onOpenOccurrence?.({
                          project: o.project,
                          collection: o.collection,
                          document: o.document,
                        })
                      }
                    >
                      <span className="flex w-full items-center gap-2">
                        <LocalePill locale={o.locale} />
                        <span className="min-w-0 flex-1 truncate">{o.snippet}</span>
                      </span>
                      <span className="font-mono text-[11px] text-muted-foreground">
                        {[o.project, o.collection, o.document].filter(Boolean).join(" / ")}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyHint title={t("No occurrence recorded here")} />
            )}
          </section>

          {data.blessings.length > 0 && (
            <>
              <Separator />
              <section data-testid="relates-blessings">
                <h4 className="mb-1 flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                  <Stamp className="size-3.5" />
                  {t("Decisions")}
                </h4>
                <ul className="divide-y">
                  {data.blessings.map((b) => (
                    <li
                      key={`${b.unit}:${b.variant ?? ""}`}
                      className="flex items-center gap-2 py-1.5"
                      data-testid="relates-blessing"
                      data-stale={b.stale}
                    >
                      <span className="min-w-0 flex-1 truncate font-mono text-[11px]">
                        {b.unit}
                      </span>
                      {b.variant && <LocalePill locale={b.variant} />}
                      {b.status && (
                        <Badge
                          variant="outline"
                          className={cn("font-normal", b.stale && "border-amber-500/40")}
                        >
                          {b.stale ? t("{status} · stale", { status: b.status }) : b.status}
                        </Badge>
                      )}
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}
        </div>
      )}
    </Pane>
  );
}

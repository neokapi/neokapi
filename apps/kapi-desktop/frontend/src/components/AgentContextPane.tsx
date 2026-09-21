// What an agent is told about a file.
//
// An agent working in this project reads the `context://<path>` resource; a
// person at a terminal runs `kapi context <path>`. One host resolution answers
// both, and this pane shows that answer for a file the reader picks: the point
// it resolves to, the voice in force, the terms that apply, and the body the
// agent receives word for word.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileSearch } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  CoordinateChip,
  EmptyState,
  ErrorNotice,
  LoadingSpinner,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { AgentContextView } from "../types/api";

export interface AgentContextPaneProps {
  tabID: string;
  /** Open on this file rather than on the picker. */
  path?: string;
}

/** How many terms to render. Zero takes the retrieval surface's own default. */
const TERM_LIMIT = 0;

export function AgentContextPane({ tabID, path }: AgentContextPaneProps) {
  const [selected, setSelected] = useState(path ?? "");

  const filesQuery = useQuery({
    queryKey: qk.contextOptions(tabID, "path"),
    queryFn: () => api.contextOptions(tabID, "path"),
  });
  const files = useMemo(() => filesQuery.data ?? [], [filesQuery.data]);

  const answerQuery = useQuery({
    queryKey: qk.agentContext(tabID, selected),
    queryFn: () => api.agentContextAt(tabID, selected, TERM_LIMIT),
    enabled: !!selected,
  });

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 border-b border-border px-6 py-3">
        <p className="mb-2 text-sm text-muted-foreground">
          Pick a file to see the context an agent receives for it, exactly as{" "}
          <code className="font-mono text-xs">kapi context</code> reports it.
        </p>
        <Select value={selected} onValueChange={setSelected}>
          <SelectTrigger size="sm" className="w-full max-w-xl" aria-label={t("Choose a file")}>
            <SelectValue placeholder={t("Choose a file")} />
          </SelectTrigger>
          <SelectContent>
            {files.map((file) => (
              <SelectItem key={file.value} value={file.value}>
                {/* A path and the collection claiming it are the project's own
                    content, so neither is translated. */}
                <span translate="no">{file.value}</span>
                {file.label && (
                  <span translate="no" className="ml-2 text-xs text-muted-foreground">
                    {file.label}
                  </span>
                )}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="px-6 py-4">
          {!selected ? (
            <EmptyState
              icon={<FileSearch size={20} />}
              title="No file chosen"
              description="Choose a file above to read what an agent is told about it."
            />
          ) : answerQuery.isLoading ? (
            <LoadingSpinner />
          ) : answerQuery.error ? (
            <ErrorNotice error={answerQuery.error} title="This file's context could not be read" />
          ) : answerQuery.data ? (
            <AgentContextBody view={answerQuery.data} />
          ) : null}
        </div>
      </ScrollArea>
    </div>
  );
}

/** The resolved answer, laid out beside the body an agent reads. */
function AgentContextBody({ view }: { view: AgentContextView }) {
  const { answer } = view;
  const terms = answer.terms ?? [];
  const coordinates = Object.entries(answer.point.coordinates ?? {});
  // Empty and thin are the two states a reader has to be able to tell apart: a
  // point with neither a voice nor terms governs nothing, and one with only
  // half of that is governed in part.
  const empty = !answer.voice && terms.length === 0;

  return (
    <div className="space-y-5">
      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Point
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          {answer.point.ref ? (
            <Badge variant="secondary" className="font-mono text-xs">
              {answer.point.ref}
            </Badge>
          ) : (
            <Badge variant="outline">The project&apos;s default point</Badge>
          )}
          {answer.point.collection && (
            <span className="text-xs text-muted-foreground">in {answer.point.collection}</span>
          )}
          {coordinates.map(([axis, value]) => (
            <CoordinateChip key={axis} axis={axis} value={value} />
          ))}
        </div>
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Voice
        </h3>
        {answer.voice ? (
          <div>
            <div className="flex items-baseline gap-2">
              <span className="text-sm font-medium">{answer.voice.name}</span>
              {answer.voice.field && (
                <span className="font-mono text-xs text-muted-foreground">
                  {answer.voice.field}
                </span>
              )}
            </div>
            {answer.voice.source && (
              <div className="truncate font-mono text-[11px] text-muted-foreground/70">
                {answer.voice.source}
              </div>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No voice profile is bound here, so an agent gets no tone or style guidance for this
            file.
          </p>
        )}
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Terms
        </h3>
        {terms.length > 0 ? (
          <>
            <ul className="space-y-1">
              {terms.map((term) => (
                <li
                  key={`${term.concept_id}:${term.term}:${term.locale ?? ""}`}
                  className="flex flex-wrap items-baseline gap-2 text-sm"
                >
                  <span className="font-mono" translate="no">
                    {term.term}
                  </span>
                  {term.discouraged ? (
                    <Badge variant="outline" className="text-destructive">
                      Discouraged
                    </Badge>
                  ) : (
                    term.status && (
                      <Badge variant="outline" className="text-muted-foreground">
                        {term.status}
                      </Badge>
                    )
                  )}
                  {term.replacement && (
                    <span className="text-xs text-muted-foreground">
                      say{" "}
                      <span className="font-mono" translate="no">
                        {term.replacement}
                      </span>{" "}
                      instead
                    </span>
                  )}
                </li>
              ))}
            </ul>
            {answer.terms_total != null && answer.terms_total > terms.length && (
              <p className="mt-2 text-xs text-muted-foreground">
                Showing {terms.length} of {answer.terms_total} terms bound here.
              </p>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            No terms are bound here, so terminology is not consulted for this file.
          </p>
        )}
      </section>

      {empty && (
        <p data-testid="agent-context-empty" className="text-sm text-muted-foreground">
          Nothing governs this file yet. An agent editing it works from the file alone.
        </p>
      )}

      {(answer.notes?.length ?? 0) > 0 && (
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Caveats
          </h3>
          <ul className="space-y-1 text-xs text-muted-foreground">
            {answer.notes?.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        </section>
      )}

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          What the agent reads
        </h3>
        <pre
          data-testid="agent-context-text"
          className="overflow-x-auto whitespace-pre-wrap rounded-md border border-border/60 bg-muted/40 p-3 font-mono text-xs"
        >
          {view.text}
        </pre>
      </section>
    </div>
  );
}

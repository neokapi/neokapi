// By-content retrieval, grouped by kind (Apache-2.0).
//
// Deliberately NOT one ranked list. A term match scores on lexical closeness to
// a concept; a precedent match scores on segment similarity. The two numbers are
// not comparable, so blending them would impose an order that means nothing.
// Each group keeps its own ranking and its own heading, and the reader weighs
// the kinds.

import { useState } from "react";
import type { FormEvent } from "react";
import { Badge, Button, Input, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { EmptyHint, LocalePill, useResource } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { Search } from "lucide-react";
import { useScope } from "./ScopeProvider";
import { NoteStrip, Pane, PaneError, PaneSkeleton, ReachNote, ValidityChip } from "./atoms";
import { SEARCH_GROUP_ORDER } from "./types";
import type { RelationSubject, SearchAnswer, SearchGroup } from "./types";

export interface ContextSearchPanelProps {
  /** Selecting a result makes it the subject of "how it relates". */
  onSelect?: (subject: RelationSubject) => void;
  limit?: number;
  className?: string;
}

function groupLabel(kind: SearchGroup["kind"]): string {
  switch (kind) {
    case "terms":
      return t("What we call it");
    case "precedent":
      return t("How we have said it");
    case "profiles":
      return t("What governs it");
    case "content":
      return t("Where it appears");
  }
}

function groupCount(group: SearchGroup): number {
  return group.hits.length;
}

function GroupBody({
  group,
  onSelect,
}: {
  group: SearchGroup;
  onSelect?: (subject: RelationSubject) => void;
}) {
  switch (group.kind) {
    case "terms":
      return (
        <ul className="divide-y">
          {group.hits.map((hit) => (
            <li key={`${hit.concept_id}:${hit.term}:${hit.locale ?? ""}`}>
              <button
                type="button"
                className="flex w-full items-center gap-2 py-1.5 text-left hover:text-primary"
                data-testid="search-hit"
                onClick={() => onSelect?.({ kind: "concept", id: hit.concept_id, label: hit.term })}
              >
                <span
                  className={cn("font-medium", hit.discouraged && "text-destructive line-through")}
                >
                  {hit.term}
                </span>
                {hit.locale && <LocalePill locale={hit.locale} />}
                {hit.discouraged && hit.replacement && (
                  <span className="text-muted-foreground">
                    {t("use {replacement}", { replacement: hit.replacement })}
                  </span>
                )}
                <ValidityChip from={hit.valid_from} to={hit.valid_to} />
                {hit.uses !== undefined && (
                  // The count is the context graph's, written at extraction, and
                  // the tooltip says so: a reader comparing it against a file
                  // just edited has to know which of the two moved.
                  <SimpleTooltip content={t("as of the last extraction")}>
                    <span
                      className="ml-auto text-xs tabular-nums text-muted-foreground"
                      data-testid="search-hit-uses"
                    >
                      {t("{count} uses", { count: hit.uses })}
                    </span>
                  </SimpleTooltip>
                )}
              </button>
            </li>
          ))}
        </ul>
      );
    case "precedent":
      return (
        <ul className="divide-y">
          {group.hits.map((hit) => (
            <li key={hit.entry_id} className="py-1.5" data-testid="search-hit">
              <span className="flex items-start gap-2">
                {hit.locale && <LocalePill locale={hit.locale} />}
                <span className="min-w-0 flex-1">{hit.text}</span>
              </span>
              {hit.discouraged && hit.discouraged.length > 0 && (
                <p className="mt-0.5 text-xs text-destructive">
                  {t("contains discouraged wording: {terms}", {
                    terms: hit.discouraged.join(", "),
                  })}
                </p>
              )}
            </li>
          ))}
        </ul>
      );
    case "profiles":
      return (
        <ul className="flex flex-wrap gap-2">
          {group.hits.map((hit) => (
            <li key={hit.name} className="flex items-center gap-1.5" data-testid="search-hit">
              <span className="text-xs font-medium">{hit.name}</span>
              <ValidityChip state={hit.state} from={hit.valid_from} to={hit.valid_to} />
            </li>
          ))}
        </ul>
      );
    case "content":
      return (
        <ul className="divide-y">
          {group.hits.map((hit) => (
            <li key={hit.id}>
              <button
                type="button"
                className="flex w-full flex-col items-start py-1.5 text-left hover:text-primary"
                data-testid="search-hit"
                onClick={() => onSelect?.({ kind: "block", id: hit.id, label: hit.text })}
              >
                <span className="flex w-full items-center gap-2">
                  {hit.locale && <LocalePill locale={hit.locale} />}
                  <span className="min-w-0 flex-1 truncate">{hit.text ?? hit.id}</span>
                </span>
                {hit.document && (
                  <span className="font-mono text-[11px] text-muted-foreground">
                    {hit.document}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      );
  }
}

export function ContextSearchPanel({ onSelect, limit, className }: ContextSearchPanelProps) {
  const { scope, source, capabilities } = useScope();
  const [draft, setDraft] = useState("");
  const [query, setQuery] = useState("");

  const { data, loading, error } = useResource<SearchAnswer | null>(
    () => (source.search && query ? source.search(query, { scope, limit }) : null),
    [query, scope.workspace, scope.project, scope.stream, scope.collection, scope.locale, limit],
  );

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setQuery(draft.trim());
  };

  if (!capabilities.search) return null;

  const groups = data
    ? [...data.groups].sort(
        (a, b) => SEARCH_GROUP_ORDER.indexOf(a.kind) - SEARCH_GROUP_ORDER.indexOf(b.kind),
      )
    : [];
  const found = groups.reduce((n, g) => n + groupCount(g), 0);

  return (
    <Pane
      title={t("Search this scope")}
      icon={<Search />}
      reach={data ? data.scope : undefined}
      className={className}
      data-testid="search-panel"
      actions={
        <form onSubmit={submit} className="flex items-center gap-1.5">
          <Input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={t("a word or phrase")}
            aria-label={t("Search this scope")}
            className="h-8 w-56"
            data-testid="search-input"
          />
          <Button type="submit" size="sm" variant="secondary">
            {t("Search")}
          </Button>
        </form>
      }
    >
      {!query && (
        <ReachNote>
          {t("Ask what the project calls something, and how it has said it before.")}
        </ReachNote>
      )}
      {query && error && <PaneError error={error} />}
      {query && !error && loading && <PaneSkeleton rows={4} />}
      {query && !error && !loading && data && (
        <div className="space-y-4 text-sm" data-testid="search-results">
          <NoteStrip notes={data.notes} />
          {found === 0 ? (
            <EmptyHint
              title={t("Nothing matched {query}", { query: data.query })}
              description={t("The scope searched was: {scope}.", { scope: data.scope })}
            />
          ) : (
            groups
              .filter((g) => groupCount(g) > 0)
              .map((group) => (
                <section key={group.kind} data-testid="search-group" data-kind={group.kind}>
                  <h4 className="mb-1 flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                    {groupLabel(group.kind)}
                    <Badge variant="secondary" className="font-normal">
                      {groupCount(group)}
                    </Badge>
                  </h4>
                  <GroupBody group={group} onSelect={onSelect} />
                </section>
              ))
          )}
        </div>
      )}
    </Pane>
  );
}

// ConceptTimeline — the concept's evolution panel (Apache-2.0). It loads the
// concept's relations, neighbour labels, the platform revision log (when the
// source has one), and named markets; projects them into an `EvolutionModel`
// (`./evolution-model`); and renders the responsive `ConceptEvolution` view —
// the horizontal language-lane roadmap, folding to a vertical git-graph when the
// panel is narrow. With no revision log the model is synthesised from term and
// relation validity windows, so the same panel works against a local SQLite
// termbase and against bowrain's REST API.
//
// Bind it into a ConceptView timeline slot:  slots={{ timeline: (p) => <ConceptTimeline {...p} /> }}
import { useMemo, useState } from "react";
import { ToggleGroup, ToggleGroupItem } from "@neokapi/ui-primitives";
import { History } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import { ConceptSection, EmptyHint, ErrorHint } from "./atoms";
import { primaryName } from "./concept-meta";
import { ConceptEvolution } from "./ConceptEvolution";
import { buildEvolutionModel } from "./evolution-model";
import type { EvolutionOrder } from "./evolution-view";
import { useResource } from "./useResource";

export function ConceptTimeline({
  concept,
  source,
  capabilities,
  onNavigate,
}: ConceptSectionProps) {
  const [order, setOrder] = useState<EvolutionOrder>("desc");
  // A stable "now" for this panel's lifetime — the open end of open spans and the
  // axis end. Kept out of the pure model so the model stays deterministic.
  const now = useMemo(() => new Date().toISOString(), []);

  // Direct relations: needed for sibling/rename/relation events and labels.
  const rel = useResource(() => source.getRelations(concept.id), [source, concept.id]);
  const relations = useMemo(() => rel.data ?? [], [rel.data]);

  const neighbourIds = useMemo(() => {
    const ids = new Set<string>();
    for (const r of relations) {
      const other = r.sourceId === concept.id ? r.targetId : r.sourceId;
      if (other !== concept.id) ids.add(other);
    }
    return [...ids];
  }, [relations, concept.id]);

  // Neighbour labels for rename/relation milestones.
  const names = useResource(async () => {
    const entries = await Promise.all(
      neighbourIds.map(async (id) => {
        const s = source.getConceptSummary
          ? await source.getConceptSummary(id)
          : await source.getConcept(id);
        return [id, s ? primaryName(s) : id] as const;
      }),
    );
    return Object.fromEntries(entries) as Record<string, string>;
  }, [source, neighbourIds.join(",")]);

  // The platform revision log + named markets, when the source supplies them.
  const remote = useResource(
    () => (capabilities.timeline && source.getTimeline ? source.getTimeline(concept.id) : []),
    [source, concept.id, capabilities.timeline],
  );
  const markets = useResource(
    () => (capabilities.markets && source.getMarkets ? source.getMarkets() : []),
    [source, capabilities.markets],
  );

  const model = useMemo(
    () =>
      buildEvolutionModel(
        {
          concept,
          relations,
          neighbourLabels: names.data ?? {},
          timeline: remote.data ?? [],
          markets: markets.data ?? [],
        },
        { now },
      ),
    [concept, relations, names.data, remote.data, markets.data, now],
  );

  // Surface a failed fetch rather than letting it read as an empty history.
  const error = rel.error ?? remote.error;

  return (
    <ConceptSection
      title="Timeline"
      icon={<History />}
      description="How this concept evolved — terms, renames, and reach over time."
      actions={
        model.eventCount > 1 ? (
          <ToggleGroup
            type="single"
            size="sm"
            value={order}
            onValueChange={(v) => v && setOrder(v as EvolutionOrder)}
            aria-label="Timeline order"
          >
            <ToggleGroupItem value="desc" className="px-2 text-xs">
              Newest
            </ToggleGroupItem>
            <ToggleGroupItem value="asc" className="px-2 text-xs">
              Oldest
            </ToggleGroupItem>
          </ToggleGroup>
        ) : undefined
      }
    >
      {error ? (
        <ErrorHint title="Could not load timeline" description={error.message} />
      ) : model.eventCount === 0 ? (
        <EmptyHint
          icon={<History />}
          title="No history yet"
          description="This concept has no recorded changes."
        />
      ) : (
        <ConceptEvolution model={model} order={order} onNavigate={onNavigate} />
      )}

      {!error && model.derived && model.eventCount > 0 && (
        <p className="mt-4 border-t pt-3 text-[11px] text-muted-foreground">
          Derived from the local termbase — status and relation changes inferred from validity
          windows.
        </p>
      )}
    </ConceptSection>
  );
}

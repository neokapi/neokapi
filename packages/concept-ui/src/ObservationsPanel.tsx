// ObservationsPanel — the "what others say" axis (Apache-2.0). External evidence
// attached to a concept: a competitor's phrasing, customer language, a
// style-guide citation, a regulatory note. Evidence, not rules — it informs but
// enforces nothing. Rendered only when the source supplies observations.
import { Skeleton } from "@neokapi/ui-primitives";
import { MessageSquareQuote } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import type { Observation } from "./types";
import { ConceptSection, EmptyHint, ErrorHint, LocalePill, formatDate } from "./atoms";
import { useResource } from "./useResource";

const KIND_LABEL: Record<string, string> = {
  competitor: "competitor",
  customer: "customer",
  style_guide: "style guide",
  regulatory: "regulatory",
  web: "web",
  internal: "internal",
};

export function ObservationsPanel({ concept, source, capabilities }: ConceptSectionProps) {
  const obs = useResource<Observation[]>(
    () =>
      capabilities.observations && source.getObservations ? source.getObservations(concept.id) : [],
    [source, concept.id, capabilities.observations],
  );
  const loading = obs.loading && !obs.data;

  return (
    <ConceptSection
      title="Observations"
      icon={<MessageSquareQuote />}
      description="What others say — external evidence, not rules."
    >
      {obs.error ? (
        <ErrorHint title="Could not load observations" description={obs.error.message} />
      ) : loading ? (
        <div className="space-y-2">
          <Skeleton className="h-12 w-full rounded-lg" />
          <Skeleton className="h-12 w-full rounded-lg" />
        </div>
      ) : (obs.data?.length ?? 0) === 0 ? (
        <EmptyHint
          icon={<MessageSquareQuote />}
          title="No observations yet"
          description="Record how competitors, customers, or style guides use this concept."
        />
      ) : (
        <ul className="space-y-2.5">
          {obs.data!.map((o) => (
            <ObservationRow key={o.id} observation={o} />
          ))}
        </ul>
      )}
    </ConceptSection>
  );
}

function ObservationRow({ observation: o }: { observation: Observation }) {
  return (
    <li className="rounded-lg border bg-card p-3">
      <div className="mb-1.5 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
        <span className="rounded bg-muted px-1.5 py-0.5 font-medium capitalize text-foreground">
          {KIND_LABEL[o.kind] ?? o.kind}
        </span>
        {o.locale && <LocalePill locale={o.locale} />}
        {o.market && <span className="capitalize">{o.market}</span>}
        <span className="ml-auto">{o.source}</span>
      </div>
      <blockquote className="border-l-2 pl-2.5 text-sm italic text-foreground">
        &ldquo;{o.quote}&rdquo;
      </blockquote>
      {o.note && <p className="mt-1 text-[11px] text-muted-foreground">{o.note}</p>}
      {(o.actor || o.at) && (
        <p className="mt-1.5 text-[11px] text-muted-foreground">
          {o.actor}
          {o.actor && o.at ? " · " : ""}
          {formatDate(o.at)}
        </p>
      )}
    </li>
  );
}

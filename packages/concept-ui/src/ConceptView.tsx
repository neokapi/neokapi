// ConceptView — the per-concept dashboard (Apache-2.0). The frame that tells one
// concept's whole story: a header (name, domain, definition, source) and a
// responsive layout of SECTION SLOTS the panel agents fill — the three axes
// (geography market panels, an evolution timeline, validity constraints), a
// LOCAL relations widget (this concept + its direct relations only), and the
// optional rich sections (observations, comments).
//
// The shell owns only the frame and the data-source plumbing: it loads the
// concept, resolves capabilities, and hands every section the same context. A
// slot that isn't supplied renders a labelled placeholder so the intended
// layout reads now and the panels can land independently.
import { useMemo } from "react";
import type { ReactNode } from "react";
import { Button, Badge, Skeleton, cn } from "@neokapi/ui-primitives";
import {
  ArrowLeft,
  Pencil,
  Globe,
  Share2,
  History,
  CalendarClock,
  Quote,
  MessageSquare,
} from "lucide-react";
import type { ConceptCapabilities, ConceptDataSource } from "./adapter";
import { resolveCapabilities } from "./adapter";
import type { Concept, TermSource } from "./types";
import { primaryName } from "./concept-meta";
import { termsByLocale } from "./grouping";
import { ConceptSection, EmptyHint, formatRelative } from "./atoms";
import { useResource } from "./useResource";

const SOURCE_LABEL: Record<TermSource, string> = {
  terminology: "Terminology",
  brand_vocabulary: "Brand vocabulary",
};

// ── Section-slot API ─────────────────────────────────────────────────────────

/**
 * The context every section slot receives. Panels read `concept` for the loaded
 * subject, `source` for their own (optional) reads/mutations, `capabilities` to
 * gate affordances, and `onNavigate` to re-centre the view on another concept.
 */
export interface ConceptSectionProps {
  concept: Concept;
  source: ConceptDataSource;
  capabilities: ConceptCapabilities;
  /** Re-centre on another concept (e.g. a relation target). */
  onNavigate: (conceptId: string) => void;
}

/**
 * A section renderer. It returns the COMPLETE section (use the exported
 * `ConceptSection` frame for consistent chrome) — the shell places it as-is.
 */
export type ConceptSectionRenderer = (props: ConceptSectionProps) => ReactNode;

/**
 * The fillable regions of a concept view. The four canonical regions (geography,
 * relations, timeline, constraints) render a labelled placeholder when their
 * slot is absent; the optional rich regions (observations, comments) render
 * nothing when absent.
 */
export interface ConceptViewSlots {
  /** Geography — market/region panels: locales, term, and status per market. */
  geography?: ConceptSectionRenderer;
  /** Relations — the LOCAL graph widget: this concept + its direct relations. */
  relations?: ConceptSectionRenderer;
  /** Timeline — the concept's evolution as a vertical timeline. */
  timeline?: ConceptSectionRenderer;
  /** Constraints — validity windows and where a term is banned/preferred. */
  constraints?: ConceptSectionRenderer;
  /** Observations — external evidence (optional, rich). */
  observations?: ConceptSectionRenderer;
  /** Comments — threaded discussion (optional, rich). */
  comments?: ConceptSectionRenderer;
}

export interface ConceptViewProps {
  conceptId: string;
  source: ConceptDataSource;
  /** Re-centre on another concept (relation navigation). */
  onNavigate: (conceptId: string) => void;
  /** Back to the list. When omitted, the back control is hidden. */
  onBack?: () => void;
  /** Edit the concept. Shown only when the source supports editing. */
  onEdit?: (concept: Concept) => void;
  /** The section renderers that fill the layout. */
  slots?: ConceptViewSlots;
  className?: string;
}

export function ConceptView({
  conceptId,
  source,
  onNavigate,
  onBack,
  onEdit,
  slots = {},
  className,
}: ConceptViewProps) {
  const caps = useMemo(() => resolveCapabilities(source), [source]);
  const { data: concept, loading } = useResource(
    () => source.getConcept(conceptId),
    [source, conceptId],
  );

  if (loading && !concept) return <ConceptViewSkeleton onBack={onBack} className={className} />;

  if (!concept) {
    return (
      <div className={cn("flex flex-col gap-4", className)}>
        {onBack && <BackButton onBack={onBack} />}
        <EmptyHint title="Concept not found" description="It may have been deleted or merged." />
      </div>
    );
  }

  const ctx: ConceptSectionProps = { concept, source, capabilities: caps, onNavigate };
  const localeCount = termsByLocale(concept.terms).length;
  const canEdit = Boolean(onEdit) && (caps.editTerms || caps.editRelations);

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      <ConceptHeader
        concept={concept}
        localeCount={localeCount}
        onBack={onBack}
        onEdit={canEdit ? () => onEdit!(concept) : undefined}
      />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
        <div className="flex min-w-0 flex-col gap-4">
          <Region
            slot={slots.geography}
            ctx={ctx}
            title="Geography"
            icon={<Globe />}
            description="Markets and the term and status used in each."
          />
          <Region
            slot={slots.constraints}
            ctx={ctx}
            title="Constraints"
            icon={<CalendarClock />}
            description="Validity windows and where a term is banned or preferred."
          />
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Region
            slot={slots.relations}
            ctx={ctx}
            title="Relations"
            icon={<Share2 />}
            description="This concept and its direct relations."
          />
        </div>
      </div>

      {/* Evolution timeline — full width so the horizontal roadmap has room; it
          folds to the vertical git-graph when the container is narrow. */}
      <Region
        slot={slots.timeline}
        ctx={ctx}
        title="Timeline"
        icon={<History />}
        description="How this concept evolved."
      />

      {/* Optional rich regions — rendered only when supplied. */}
      {(slots.observations || slots.comments) && (
        <div className="grid gap-4 lg:grid-cols-2">
          {slots.observations && (
            <Region slot={slots.observations} ctx={ctx} title="Observations" icon={<Quote />} />
          )}
          {slots.comments && (
            <Region slot={slots.comments} ctx={ctx} title="Comments" icon={<MessageSquare />} />
          )}
        </div>
      )}
    </div>
  );
}

// ── Header ───────────────────────────────────────────────────────────────────

function ConceptHeader({
  concept,
  localeCount,
  onBack,
  onEdit,
}: {
  concept: Concept;
  localeCount: number;
  onBack?: () => void;
  onEdit?: () => void;
}) {
  const name = primaryName(concept);
  return (
    <header className="flex flex-col gap-3">
      {onBack && <BackButton onBack={onBack} />}
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            {concept.domain && (
              <Badge variant="outline" className="font-normal">
                {concept.domain}
              </Badge>
            )}
            {concept.source && <span>{SOURCE_LABEL[concept.source]}</span>}
            {concept.updatedAt && (
              <>
                <span aria-hidden>·</span>
                <span>Updated {formatRelative(concept.updatedAt)}</span>
              </>
            )}
          </div>
          <h1 className="text-2xl font-semibold leading-tight tracking-tight text-foreground">
            {name}
          </h1>
          {concept.definition && (
            <p className="max-w-2xl text-sm leading-relaxed text-muted-foreground">
              {concept.definition}
            </p>
          )}
          <div className="flex items-center gap-4 pt-0.5 text-xs text-muted-foreground">
            <Stat value={concept.terms.length} unit="term" />
            <Stat value={localeCount} unit="locale" />
          </div>
        </div>
        {onEdit && (
          <Button variant="outline" size="sm" onClick={onEdit} className="shrink-0">
            <Pencil />
            Edit
          </Button>
        )}
      </div>
    </header>
  );
}

function Stat({ value, unit }: { value: number; unit: string }) {
  return (
    <span>
      <span className="font-medium text-foreground">{value}</span> {unit}
      {value === 1 ? "" : "s"}
    </span>
  );
}

function BackButton({ onBack }: { onBack: () => void }) {
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={onBack}
      className="-ml-2 w-fit text-muted-foreground"
    >
      <ArrowLeft />
      Concepts
    </Button>
  );
}

// ── Region (slot or placeholder) ─────────────────────────────────────────────

function Region({
  slot,
  ctx,
  title,
  icon,
  description,
}: {
  slot: ConceptSectionRenderer | undefined;
  ctx: ConceptSectionProps;
  title: string;
  icon: ReactNode;
  description?: string;
}) {
  if (slot) return <>{slot(ctx)}</>;
  return (
    <ConceptSection title={title} icon={icon} description={description}>
      <div className="rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center text-xs text-muted-foreground">
        This section lands next.
      </div>
    </ConceptSection>
  );
}

// ── Skeleton ─────────────────────────────────────────────────────────────────

function ConceptViewSkeleton({ onBack, className }: { onBack?: () => void; className?: string }) {
  return (
    <div className={cn("flex flex-col gap-5", className)}>
      {onBack && <BackButton onBack={onBack} />}
      <div className="space-y-2">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-7 w-56" />
        <Skeleton className="h-4 w-96 max-w-full" />
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
        <Skeleton className="h-48 w-full rounded-xl" />
        <Skeleton className="h-48 w-full rounded-xl" />
      </div>
    </div>
  );
}

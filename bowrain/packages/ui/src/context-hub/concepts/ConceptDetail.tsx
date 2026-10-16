// The selected concept in the Concepts detail pane: its term and status in the
// toolbar with the acts open on it, then its definition, the forms it
// discourages and what to write instead, its terms language by language, where
// it came from and who changed it, its relations, and the platform's record.
//
// The acts are the platform's: Approve sets a proposed headline term to
// approved, Relate relates the concept to another, and Edit sets each term's
// status. An ordinary change lands at once. A governed one (banning or
// promoting a term, a replaced-by relation) is refused by the server and opens
// the change-set prompt the page holds.
import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import type { ConceptDataSource, ConceptNaming, ConceptSummary, Term } from "@neokapi/concept-ui";
import { primaryName } from "@neokapi/concept-ui";
import {
  ActorMark,
  Button,
  CoordinateChip,
  ListDetailEmpty,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  LocaleLabel,
  When,
} from "@neokapi/ui-primitives";
import { Check, Pencil, Plus } from "../../components/icons";
import { ErrorNotice } from "../../errors";
import { useAnalytics } from "../../context/AnalyticsContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { AnalyticsEvents } from "../../analytics-events";
import {
  invalidateConceptAggregates,
  useConcept,
  useConceptStory,
} from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { useProjects } from "../../hooks/useProjectApi";
import type { Market, ConceptStoryEntry } from "../../types/brand-graph";
import { conceptFromInfo, isGovernedEditError } from "./restConceptSource";
import {
  coordinatesOf,
  discouragedTerms,
  headlineTerm,
  languagesInOrder,
  marketsFor,
  provenanceOf,
  replacementFor,
} from "./conceptFacts";
import { ConceptEditDialog } from "./ConceptEditDialog";
import { ConceptRecord } from "./ConceptRecord";
import { ConceptRelations, RelateDialog } from "./ConceptRelations";
import { TermStatusChip } from "./TermStatusChip";
import type { GovernedEditError } from "./restConceptSource";

export interface ConceptDetailProps {
  conceptId: string;
  /** The concept as the list read it, shown while the full record is read. */
  summary?: ConceptSummary;
  /** The governance-aware source the edit dialog and Approve write through. */
  source: ConceptDataSource;
  naming: ConceptNaming;
  /** The workspace's named markets, which each language is labelled with. */
  markets?: Market[];
  /** Open another concept in the pane. */
  onNavigate: (conceptId: string) => void;
  /** The server refused an edit as governed. */
  onGoverned: (error: GovernedEditError) => void;
}

export function ConceptDetail({
  conceptId,
  summary,
  source,
  naming,
  markets,
  onNavigate,
  onGoverned,
}: ConceptDetailProps) {
  const qc = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const { capture } = useAnalytics();
  const full = useConcept(conceptId);
  const story = useConceptStory(conceptId);
  const [editing, setEditing] = useState(false);
  const [relating, setRelating] = useState(false);

  const refresh = () => {
    invalidateConceptAggregates(qc, ws);
    void qc.invalidateQueries({ queryKey: ["concept", ws, conceptId] });
    void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
  };

  const approve = useMutation({
    mutationFn: async (term: Term) => {
      if (!source.setTermStatus) return;
      await source.setTermStatus(conceptId, { locale: term.locale, text: term.text }, "approved");
    },
    onSuccess: (_r, term) => {
      capture(AnalyticsEvents.glossarySaved, { status: "approved", locale: term.locale });
      refresh();
    },
  });

  // The list's copy stands in while the full record is read.
  const concept = useMemo(
    () => (full.data ? conceptFromInfo(full.data) : summary),
    [full.data, summary],
  );

  if (!concept) {
    if (full.isLoading) {
      return (
        <div className="p-6">
          <LoadingSpinner />
        </div>
      );
    }
    if (full.error && !isNotFound(full.error)) {
      return (
        <div className="p-6">
          <ErrorNotice error={full.error} title={t("This concept could not be read")} />
        </div>
      );
    }
    return (
      <ListDetailEmpty
        title={t("This concept is no longer in the workspace")}
        description={t("It may have been deleted or merged.")}
      />
    );
  }

  const name = primaryName(concept, naming);
  const headline = headlineTerm(concept, naming);
  const discouraged = discouragedTerms(concept);
  const languages = languagesInOrder(concept, naming.sourceLocale);
  const coordinates = coordinatesOf(full.data?.properties);
  const profile = full.data?.properties?.profile;
  const canApprove = headline?.status === "proposed" && !!source.setTermStatus;
  const approveError =
    approve.error && !isGovernedEditError(approve.error) ? approve.error : undefined;

  return (
    <ListDetailPane
      data-slot="concept-detail"
      data-testid="concept-view"
      data-concept={concept.id}
      toolbar={
        <ListDetailToolbar
          data-testid="concept-header"
          title={
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate" translate="no">
                {name}
              </span>
              {headline && <TermStatusChip status={headline.status} />}
            </span>
          }
          actions={
            <>
              {canApprove && headline && (
                <Button
                  size="sm"
                  variant="success"
                  data-slot="concept-approve"
                  disabled={approve.isPending}
                  onClick={() => approve.mutate(headline)}
                >
                  <Check size={13} />
                  Approve
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                data-slot="concept-relate"
                onClick={() => setRelating(true)}
              >
                <Plus size={13} />
                Relate
              </Button>
              <Button
                variant="outline"
                size="sm"
                data-slot="concept-edit"
                disabled={!full.data}
                onClick={() => setEditing(true)}
              >
                <Pencil size={13} />
                Edit
              </Button>
            </>
          }
        />
      }
    >
      {approveError ? (
        <ErrorNotice
          variant="inline"
          error={approveError}
          title={t("The term was not approved")}
          hint={t("Approving a term needs permission to edit terms in this workspace.")}
        />
      ) : null}

      {concept.definition && (
        <p className="max-w-prose text-sm" translate="no">
          {concept.definition}
        </p>
      )}

      {discouraged.length > 0 && (
        <section data-slot="concept-discouraged" aria-label={t("Discouraged forms")}>
          <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Write instead</h3>
          <ul className="space-y-1">
            {discouraged.map((term) => {
              const instead = replacementFor(term, concept.terms);
              return (
                <li
                  key={`${term.locale}:${term.text}`}
                  className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm"
                >
                  <span className="line-through decoration-muted-foreground" translate="no">
                    {term.text}
                  </span>
                  {instead && (
                    <span className="text-muted-foreground">
                      use{" "}
                      <span className="text-foreground" translate="no">
                        {instead.text}
                      </span>
                    </span>
                  )}
                  <TermStatusChip status={term.status} />
                  <LocaleLabel locale={term.locale} compact className="text-[11px]" />
                  {term.note && (
                    <span className="basis-full text-xs text-muted-foreground" translate="no">
                      {term.note}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        </section>
      )}

      {languages.length > 0 && (
        <section data-slot="concept-languages" aria-label={t("Terms by language")}>
          <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">By language</h3>
          <dl className="grid grid-cols-[minmax(7rem,max-content)_minmax(0,1fr)] gap-x-4 gap-y-1.5">
            {languages.map((group) => {
              const where = marketsFor(group.locale, markets);
              return (
                <div key={group.locale} className="contents">
                  <dt className="pt-0.5 text-xs">
                    <LocaleLabel
                      locale={group.locale}
                      source={group.locale === naming.sourceLocale}
                      variant="short"
                    />
                    {where.length > 0 && (
                      <span
                        data-slot="concept-language-markets"
                        className="block text-[11px] text-muted-foreground"
                        translate="no"
                      >
                        {where.join(", ")}
                      </span>
                    )}
                  </dt>
                  <dd className="min-w-0 space-y-1">
                    {group.terms.map((term) => (
                      <TermLine key={`${term.locale}:${term.text}`} term={term} />
                    ))}
                  </dd>
                </div>
              );
            })}
          </dl>
        </section>
      )}

      <Provenance
        domain={concept.domain}
        profile={profile}
        coordinates={coordinates}
        projectId={full.data?.project_id}
        createdAt={full.data?.created_at}
        updatedAt={full.data?.updated_at}
        story={story.data?.entries}
      />

      <ConceptRelations concept={concept} naming={naming} onNavigate={onNavigate} canEdit />

      <ConceptRecord conceptId={concept.id} />

      {full.data && (
        <ConceptEditDialog
          concept={conceptFromInfo(full.data)}
          source={source}
          open={editing}
          onOpenChange={setEditing}
          onApplied={refresh}
        />
      )}
      <RelateDialog
        concept={concept}
        naming={naming}
        open={relating}
        onOpenChange={setRelating}
        onGoverned={onGoverned}
      />
    </ListDetailPane>
  );
}

function isNotFound(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /\b404\b/.test(message) || /not found/i.test(message);
}

/** One term in its language: the text, its status, its other forms and its grammar. */
function TermLine({ term }: { term: Term }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm">
      <span translate="no">{term.text}</span>
      <TermStatusChip status={term.status} />
      {term.forms && term.forms.length > 0 && (
        <span className="text-xs text-muted-foreground">
          also <span translate="no">{term.forms.join(", ")}</span>
        </span>
      )}
      {(term.partOfSpeech || term.gender) && (
        <span className="text-[11px] text-muted-foreground" translate="no">
          {[term.partOfSpeech, term.gender].filter(Boolean).join(", ")}
        </span>
      )}
      {term.note && (
        <span className="basis-full text-xs text-muted-foreground" translate="no">
          {term.note}
        </span>
      )}
    </div>
  );
}

/** Where a concept came from: its domain, profile and point, its project, and who added and changed it. */
function Provenance({
  domain,
  profile,
  coordinates,
  projectId,
  createdAt,
  updatedAt,
  story,
}: {
  domain?: string;
  profile?: string;
  coordinates: Array<[string, string]>;
  projectId?: string;
  createdAt?: string;
  updatedAt?: string;
  story?: ConceptStoryEntry[];
}) {
  const { nameOf } = useUserDisplayNames();
  const { data: projects } = useProjects();
  const { added, changed } = provenanceOf(story);
  const project = projectId ? projects?.find((p) => p.id === projectId) : undefined;
  return (
    <section data-slot="concept-provenance" aria-label={t("Where it came from")}>
      <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Where it came from</h3>
      <dl className="grid grid-cols-[minmax(7rem,max-content)_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm">
        {domain && (
          <Fact label={<>Domain</>}>
            <span translate="no">{domain}</span>
          </Fact>
        )}
        {profile && (
          <Fact label={<>Voice profile</>}>
            <span className="font-mono text-xs" translate="no">
              {profile}
            </span>
          </Fact>
        )}
        {coordinates.length > 0 && (
          <Fact label={<>Applies at</>}>
            <span className="flex flex-wrap gap-1">
              {coordinates.map(([axis, value]) => (
                <CoordinateChip key={axis} axis={axis} value={value} />
              ))}
            </span>
          </Fact>
        )}
        <Fact label={<>Scope</>}>
          {projectId ? (
            <span translate="no">{project?.name ?? projectId}</span>
          ) : (
            <>Every project in the workspace</>
          )}
        </Fact>
        {createdAt && (
          <Fact label={<>Added</>}>
            <span className="inline-flex flex-wrap items-center gap-x-1.5">
              <When iso={createdAt} />
              {added?.actor && <Actor name={nameOf(added.actor)} />}
            </span>
          </Fact>
        )}
        {updatedAt && updatedAt !== createdAt && (
          <Fact label={<>Changed</>}>
            <span className="inline-flex flex-wrap items-center gap-x-1.5">
              <When iso={updatedAt} relative />
              {changed?.actor && <Actor name={nameOf(changed.actor)} />}
            </span>
          </Fact>
        )}
      </dl>
    </section>
  );
}

function Actor({ name }: { name: string }) {
  return (
    <>
      <span aria-hidden className="text-muted-foreground">
        ·
      </span>
      <span data-slot="concept-actor" className="inline-flex items-center gap-1.5">
        <ActorMark actor="person" name={name} size="sm" decorative />
        <span translate="no">{name}</span>
      </span>
    </>
  );
}

function Fact({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

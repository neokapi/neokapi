// The context explorer, driven against a workspace.
//
// The same component set kapi renders against one project on disk, with the
// dimensions free: a workspace holds every project's subgraph over one
// vocabulary, so the ladder starts above the projects and the panes answer
// across them. Nothing here is a second model — the source maps the endpoints
// the workspace already serves into the answers the components read.
//
// Governance is DERIVED at this reach, and the capability says so. A workspace
// recovers the points its content occupies by grouping what each project
// declared on push; it does not walk a recipe. A point no content has been
// pushed to is therefore invisible here and resolvable locally, which is the
// honest difference between the two mounts rather than a gap in one of them.

import type {
  ContentAnswer,
  ContextDataSource,
  Dimension,
  ExplorerCapabilities,
  GovernanceAnswer,
  RelationSubject,
  RelationsAnswer,
  RungOption,
  ScopeTuple,
  ScopedQuery,
  SearchAnswer,
  SearchGroup,
  TermHit,
} from "@neokapi/context-explorer";
import { isBannedStatus } from "@neokapi/concept-ui";
import type { TermStatus } from "@neokapi/concept-ui";
import type { ApiAdapter } from "../api/adapter";
import type { ContextProfile } from "../types/context-profiles";
import type { ConceptInfo, LocaleTranslationStats } from "../types/api";

/** The dimensions a workspace lets the reader move. */
export const WORKSPACE_FREE_DIMENSIONS: Dimension[] = [
  "project",
  "stream",
  "collection",
  "path",
  "coordinate",
  "locale",
];

const WORKSPACE_CAPABILITIES: Partial<ExplorerCapabilities> = {
  free: WORKSPACE_FREE_DIMENSIONS,
  // Recovered from what content declared, not resolved from a recipe.
  governance: "derived",
  // The workspace holds every project's subgraph over one vocabulary, so
  // "which projects use this concept" is a real question here.
  relations: "workspace",
  content: true,
  search: true,
};

/** A profile's coordinate, written the way the panes render a point. */
function pointOf(profile: ContextProfile): string | undefined {
  if (profile.channel && profile.name) return `${profile.name}/${profile.channel}`;
  return profile.channel || profile.name || undefined;
}

/**
 * The profile governing a tuple: the one whose collections sit at the point.
 *
 * A tuple naming a coordinate selects by coordinate; otherwise the collection
 * (and, when given, the project) decides. Nothing matching leaves the answer
 * on the workspace's default profile, which is what governs content nobody has
 * placed anywhere else.
 */
function profileFor(profiles: ContextProfile[], scope: ScopeTuple): ContextProfile | undefined {
  if (scope.coordinate) {
    const byPoint = profiles.find((p) => pointOf(p) === scope.coordinate);
    if (byPoint) return byPoint;
  }
  if (scope.collection) {
    // A collection is addressed by its id in the filter bar and by its name in
    // a recipe, and both reach here — the workspace's ids are not its names.
    const byCollection = profiles.find((p) =>
      p.collections.some(
        (c) =>
          (c.id === scope.collection || c.name === scope.collection) &&
          (!scope.project || c.project_id === scope.project || c.project_name === scope.project),
      ),
    );
    if (byCollection) return byCollection;
  }
  if (scope.project) {
    const byProject = profiles.find((p) =>
      p.collections.some((c) => c.project_id === scope.project || c.project_name === scope.project),
    );
    if (byProject) return byProject;
  }
  return profiles.find((p) => p.is_default);
}

/** A workspace concept, flattened into the term rows the panes render. */
function conceptTerms(concepts: ConceptInfo[]): TermHit[] {
  const hits: TermHit[] = [];
  for (const concept of concepts) {
    const terms = concept.terms ?? [];
    const preferred = terms.find((term) => term.status === "preferred");
    for (const term of terms) {
      const discouraged = isBannedStatus(term.status as TermStatus);
      hits.push({
        concept_id: concept.id,
        term: term.text,
        locale: term.locale,
        status: term.status,
        definition: concept.definition,
        domain: concept.domain,
        replacement: discouraged && preferred?.text !== term.text ? preferred?.text : undefined,
        discouraged,
      });
    }
  }
  return hits;
}

/** One locale row, with the ship state the server derived. */
function coverageRow(row: LocaleTranslationStats) {
  return {
    locale: row.locale,
    units: row.total_blocks,
    covered: row.translated_blocks,
    ship_state: row.ship_state,
  };
}

/**
 * The explorer's data source for one workspace.
 *
 * Every read is scoped by the tuple: a question with no project asked over the
 * workspace, a question with one narrowed to it. Slicing is a query — nodes
 * carry their scope as fields — so narrowing never changes which endpoint
 * answers, only what it is asked.
 */
export function createRestContextSource(api: ApiAdapter, workspaceSlug: string): ContextDataSource {
  const limitOf = (q: ScopedQuery) => q.limit ?? 25;

  /** The project a tuple names, resolved from an id or a name. */
  const projectIdFor = async (scope: ScopeTuple): Promise<string | undefined> => {
    if (!scope.project) return undefined;
    const projects = await api.listProjects(workspaceSlug);
    const match = projects.find((p) => p.id === scope.project || p.name === scope.project);
    return match?.id;
  };

  return {
    capabilities: WORKSPACE_CAPABILITIES,

    async governs(q: ScopedQuery): Promise<GovernanceAnswer> {
      const [profiles, concepts] = await Promise.all([
        api.listContextProfiles(workspaceSlug),
        api.listConcepts(workspaceSlug, { limit: limitOf(q) }),
      ]);
      const profile = profileFor(profiles.profiles, q.scope);
      const notes: string[] = [];
      if (!profile) {
        notes.push(
          "no content has been pushed to this point, so the workspace holds no profile for it",
        );
      }

      return {
        point: {
          path: q.scope.path,
          profile: profile && !profile.is_default ? profile.name : undefined,
          channel: profile?.channel,
          collection: q.scope.collection,
          default: profile?.is_default ?? true,
        },
        scope: "workspace",
        voice: profile?.voice
          ? { name: profile.voice.name, source: profile.voice.description }
          : undefined,
        terms: conceptTerms(concepts.concepts ?? []),
        terms_total: profiles.terms.concept_count,
        // Profile windows are a recipe declaration; the workspace observes
        // points, not the windows they were declared with.
        profiles: [],
        notes,
      };
    },

    async lives(q: ScopedQuery): Promise<ContentAnswer> {
      const projectId = await projectIdFor(q.scope);
      if (!projectId) {
        // Above a project the workspace answers with its projects' collections
        // rather than an item list: content sits in a project, and a flat list
        // across every project would be a report, not a place.
        const profiles = await api.listContextProfiles(workspaceSlug);
        const collections = profiles.profiles.flatMap((profile) =>
          profile.collections
            .filter((c) => !q.scope.coordinate || pointOf(profile) === q.scope.coordinate)
            .map((c) => ({
              id: c.id,
              name: c.name,
              project: c.project_name,
              stream: c.stream,
              point: pointOf(profile),
            })),
        );
        return {
          point: { collection: q.scope.collection, default: false },
          scope: "workspace",
          collections,
          items: [],
          coverage: [],
          notes: ["choose a project to see the content at this point"],
        };
      }

      const stats = await api.getTranslationDashboard(workspaceSlug, projectId, q.scope.stream, {
        itemCollection: q.scope.collection,
        itemLimit: limitOf(q),
      });
      const locale = q.scope.locale;
      const wanted = (row: LocaleTranslationStats) => !locale || row.locale === locale;

      return {
        point: { path: q.scope.path, collection: q.scope.collection, default: false },
        scope: "workspace",
        collections: stats.collection_stats.map((c) => ({
          id: c.collection_id,
          name: c.collection_name,
          project: q.scope.project,
          stream: q.scope.stream,
          point: c.channel,
          items: c.item_count,
        })),
        items: stats.item_stats
          .filter((item) => !q.scope.path || item.item_name.startsWith(q.scope.path))
          .flatMap((item) =>
            item.locales.filter(wanted).map((row) => ({
              id: `${item.item_id}:${row.locale}`,
              document: item.item_name,
              collection: item.collection_name,
              locale: row.locale,
              ship_state: row.ship_state,
            })),
          ),
        items_total: stats.item_total,
        coverage: stats.locale_stats.filter(wanted).map(coverageRow),
        notes: [],
      };
    },

    async relates(subject: RelationSubject, q: ScopedQuery): Promise<RelationsAnswer> {
      if (subject.kind !== "concept") {
        return {
          subject,
          reach: "workspace",
          occurrences: [],
          projects: [],
          blessings: [],
          notes: ["the workspace answers relations for a concept"],
        };
      }
      const usage = await api.getConceptBlastRadius(workspaceSlug, subject.id);
      const projects = usage.projects ?? [];
      const scoped = q.scope.project
        ? projects.filter(
            (p) => p.project_id === q.scope.project || p.project_name === q.scope.project,
          )
        : projects;
      const projectName = new Map(projects.map((p) => [p.project_id, p.project_name]));

      return {
        subject,
        // Two hops through the workspace's own vocabulary node: the concept's
        // uses, grouped by the project each use sits in. Projects relate by
        // co-occurrence, never by an edge between them.
        reach: "workspace",
        occurrences: (usage.samples ?? []).map((sample) => ({
          concept_id: usage.concept_id,
          term: subject.label ?? subject.id,
          locale: sample.locale,
          project: projectName.get(sample.project_id) ?? sample.project_id,
          stream: sample.stream,
          collection: sample.collection_name,
          document: sample.item_name,
          block_id: sample.block_id,
          matched: subject.label ?? subject.id,
          snippet: sample.text,
        })),
        occurrences_total: usage.occurrences,
        projects: scoped.map((p) => ({
          project: p.project_id,
          project_name: p.project_name,
          blocks: p.blocks,
          occurrences: p.occurrences,
          collections: p.collections.map((c) => c.collection_name),
        })),
        // Blessings are the unit ledger's, which the review session owns.
        blessings: [],
        notes: [],
      };
    },

    async search(query: string, q: ScopedQuery): Promise<SearchAnswer> {
      const limit = limitOf(q);
      const [concepts, memory] = await Promise.all([
        api.listConcepts(workspaceSlug, { q: query, limit }),
        api.getMemoryEntries(workspaceSlug, query, q.scope.locale ?? "", "", 0, limit),
      ]);

      const groups: SearchGroup[] = [];
      const terms = conceptTerms(concepts.concepts ?? []);
      if (terms.length) groups.push({ kind: "terms", hits: terms });
      if (memory.entries?.length) {
        groups.push({
          kind: "precedent",
          hits: memory.entries.map((entry) => ({
            entry_id: entry.id,
            text: entry.source,
            locale: entry.source_language,
          })),
        });
      }
      return { query, scope: "workspace", groups, notes: [] };
    },

    async options(scope: ScopeTuple, dimension: Dimension): Promise<RungOption[]> {
      switch (dimension) {
        case "project": {
          const projects = await api.listProjects(workspaceSlug);
          return projects.map((p) => ({ value: p.id, label: p.name }));
        }
        case "stream": {
          const projectId = await projectIdFor(scope);
          if (!projectId) return [];
          const streams = await api.listStreams(workspaceSlug, projectId);
          return streams.map((s) => ({ value: s.name }));
        }
        case "collection": {
          const projectId = await projectIdFor(scope);
          if (!projectId) {
            const profiles = await api.listContextProfiles(workspaceSlug);
            const seen = new Set<string>();
            for (const profile of profiles.profiles) {
              for (const c of profile.collections) seen.add(c.name);
            }
            return [...seen].sort().map((value) => ({ value }));
          }
          const collections = await api.listCollections(workspaceSlug, projectId, scope.stream);
          return collections.map((c) => ({ value: c.id, label: c.name }));
        }
        case "coordinate": {
          const profiles = await api.listContextProfiles(workspaceSlug);
          return profiles.profiles
            .map(pointOf)
            .filter((point): point is string => Boolean(point))
            .sort()
            .map((value) => ({ value }));
        }
        case "locale": {
          const projectId = await projectIdFor(scope);
          if (!projectId) {
            const projects = await api.listProjects(workspaceSlug);
            const seen = new Set<string>();
            for (const p of projects) for (const l of p.target_languages ?? []) seen.add(l);
            return [...seen].sort().map((value) => ({ value }));
          }
          const project = await api.getProject(workspaceSlug, projectId, scope.stream, {
            view: "summary",
          });
          return (project.target_languages ?? []).map((value) => ({ value }));
        }
        default:
          return [];
      }
    },
  };
}

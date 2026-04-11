import { useCallback, useMemo, useState } from "react";
import type { TMFacets, ImportSessionFacet } from "./types";
import { ENTITY_TYPES } from "./types";
import { Checkbox } from "../ui/checkbox";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "../ui/collapsible";
import { ScrollArea } from "../ui/scroll-area";
import { LocalePill } from "./LocalePill";
import { ChevronRight, X, Search } from "lucide-react";

// ─── Public interface ─────────────────────────────────────────────────────────

export interface FacetSelection {
  locales: string[];
  projects: string[];
  entityTypes: string[];
  sessionIds: string[];
  codeFilter: "all" | "has_codes" | "no_codes";
}

export const EMPTY_FACETS: FacetSelection = {
  locales: [],
  projects: [],
  entityTypes: [],
  sessionIds: [],
  codeFilter: "all",
};

interface TMFacetSidebarProps {
  facets: TMFacets | null;
  selection: FacetSelection;
  onSelectionChange: (selection: FacetSelection) => void;
  loading?: boolean;
}

// ─── Constants ────────────────────────────────────────────────────────────────

const LOCALE_INLINE_THRESHOLD = 6;
const LOCALE_COLLAPSED_COUNT = 5;
const SESSION_PAGE_SIZE = 8;
const PROJECT_VISIBLE_COUNT = 5;

// ─── Root component ───────────────────────────────────────────────────────────

export function TMFacetSidebar({
  facets,
  selection,
  onSelectionChange,
  loading,
}: TMFacetSidebarProps) {
  if (!facets && !loading) return null;

  const toggleLocale = useCallback(
    (locale: string) => {
      const next = selection.locales.includes(locale)
        ? selection.locales.filter((l) => l !== locale)
        : [...selection.locales, locale];
      onSelectionChange({ ...selection, locales: next });
    },
    [selection, onSelectionChange],
  );

  const toggleProject = useCallback(
    (projectId: string) => {
      const next = selection.projects.includes(projectId)
        ? selection.projects.filter((p) => p !== projectId)
        : [...selection.projects, projectId];
      onSelectionChange({ ...selection, projects: next });
    },
    [selection, onSelectionChange],
  );

  const toggleEntityType = useCallback(
    (entityType: string) => {
      const next = selection.entityTypes.includes(entityType)
        ? selection.entityTypes.filter((t) => t !== entityType)
        : [...selection.entityTypes, entityType];
      onSelectionChange({ ...selection, entityTypes: next });
    },
    [selection, onSelectionChange],
  );

  const toggleSession = useCallback(
    (sessionId: string) => {
      const next = selection.sessionIds.includes(sessionId)
        ? selection.sessionIds.filter((s) => s !== sessionId)
        : [...selection.sessionIds, sessionId];
      onSelectionChange({ ...selection, sessionIds: next });
    },
    [selection, onSelectionChange],
  );

  const setCodeFilter = useCallback(
    (filter: FacetSelection["codeFilter"]) => {
      onSelectionChange({
        ...selection,
        codeFilter: filter === selection.codeFilter ? "all" : filter,
      });
    },
    [selection, onSelectionChange],
  );

  const hasActiveFilters =
    selection.locales.length > 0 ||
    selection.projects.length > 0 ||
    selection.entityTypes.length > 0 ||
    selection.sessionIds.length > 0 ||
    selection.codeFilter !== "all";

  return (
    <div className="flex flex-col gap-0.5 text-sm">
      <div className="flex items-center justify-between mb-1 px-0.5">
        <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
          Filters
        </span>
        {hasActiveFilters && (
          <button
            onClick={() => onSelectionChange(EMPTY_FACETS)}
            className="text-[10px] text-muted-foreground hover:text-foreground transition-colors"
          >
            Clear all
          </button>
        )}
      </div>

      {/* Skeleton while facets are loading */}
      {loading && !facets && <FacetSkeleton />}

      {facets && facets.locales.length > 0 && (
        <LocalesSection
          locales={facets.locales}
          selectedLocales={selection.locales}
          onToggle={toggleLocale}
        />
      )}

      {facets && facets.projects.length > 0 && (
        <ProjectsSection
          projects={facets.projects}
          selectedProjects={selection.projects}
          onToggle={toggleProject}
        />
      )}

      {facets && facets.entity_types.length > 0 && (
        <FacetSection title="Entity Types">
          {facets.entity_types.map((et) => {
            const label = ENTITY_TYPES.find((t) => t.value === et.type)?.label ?? et.type;
            return (
              <FacetRow
                key={et.type}
                checked={selection.entityTypes.includes(et.type)}
                onCheckedChange={() => toggleEntityType(et.type)}
                label={label}
                count={et.count}
              />
            );
          })}
        </FacetSection>
      )}

      {facets && facets.import_sessions.length > 0 && (
        <SessionsSection
          sessions={facets.import_sessions}
          selectedIds={selection.sessionIds}
          onToggle={toggleSession}
        />
      )}

      {facets && (facets.has_codes > 0 || facets.no_codes > 0) && (
        <FacetSection title="Inline Codes" defaultOpen>
          <FacetRow
            checked={selection.codeFilter === "has_codes"}
            onCheckedChange={() => setCodeFilter("has_codes")}
            label="Has inline codes"
            count={facets.has_codes}
          />
          <FacetRow
            checked={selection.codeFilter === "no_codes"}
            onCheckedChange={() => setCodeFilter("no_codes")}
            label="Plain text only"
            count={facets.no_codes}
          />
        </FacetSection>
      )}
    </div>
  );
}

// ─── Languages section ────────────────────────────────────────────────────────

function LocalesSection({
  locales,
  selectedLocales,
  onToggle,
}: {
  locales: TMFacets["locales"];
  selectedLocales: string[];
  onToggle: (locale: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  const showAll = locales.length <= LOCALE_INLINE_THRESHOLD;
  const visible = showAll || expanded ? locales : locales.slice(0, LOCALE_COLLAPSED_COUNT);
  const hiddenCount = locales.length - LOCALE_COLLAPSED_COUNT;
  const hiddenSelected = expanded
    ? 0
    : locales.slice(LOCALE_COLLAPSED_COUNT).filter((l) => selectedLocales.includes(l.locale))
        .length;

  return (
    <FacetSection title="Languages" defaultOpen>
      {visible.map(({ locale, count }) => (
        <FacetRow
          key={locale}
          checked={selectedLocales.includes(locale)}
          onCheckedChange={() => onToggle(locale)}
          label={<LocalePill locale={locale} />}
          count={count}
        />
      ))}
      {!showAll && (
        <button
          onClick={() => setExpanded((e) => !e)}
          className="mt-0.5 ml-5 text-[10px] text-muted-foreground hover:text-foreground transition-colors text-left"
        >
          {expanded ? (
            "Show fewer"
          ) : (
            <>
              {hiddenCount} more
              {hiddenSelected > 0 && (
                <span className="ml-1 text-primary">({hiddenSelected} selected)</span>
              )}
            </>
          )}
        </button>
      )}
    </FacetSection>
  );
}

// ─── Projects section ─────────────────────────────────────────────────────────

function ProjectsSection({
  projects,
  selectedProjects,
  onToggle,
}: {
  projects: TMFacets["projects"];
  selectedProjects: string[];
  onToggle: (projectId: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  const showAll = projects.length <= PROJECT_VISIBLE_COUNT;
  const visible = showAll || expanded ? projects : projects.slice(0, PROJECT_VISIBLE_COUNT);
  const hiddenCount = projects.length - PROJECT_VISIBLE_COUNT;
  const hiddenSelected = expanded
    ? 0
    : projects.slice(PROJECT_VISIBLE_COUNT).filter((p) => selectedProjects.includes(p.project_id))
        .length;

  return (
    <FacetSection title="Project">
      {visible.map((p) => (
        <FacetRow
          key={p.project_id || "__none__"}
          checked={selectedProjects.includes(p.project_id)}
          onCheckedChange={() => onToggle(p.project_id)}
          label={
            <span className="truncate" title={p.project_id || "No project"}>
              {p.project_id || "No project"}
            </span>
          }
          count={p.count}
        />
      ))}
      {!showAll && (
        <button
          onClick={() => setExpanded((e) => !e)}
          className="mt-0.5 ml-5 text-[10px] text-muted-foreground hover:text-foreground transition-colors text-left"
        >
          {expanded ? (
            "Show fewer"
          ) : (
            <>
              {hiddenCount} more
              {hiddenSelected > 0 && (
                <span className="ml-1 text-primary">({hiddenSelected} selected)</span>
              )}
            </>
          )}
        </button>
      )}
    </FacetSection>
  );
}

// ─── Sessions section ─────────────────────────────────────────────────────────

function SessionsSection({
  sessions,
  selectedIds,
  onToggle,
}: {
  sessions: ImportSessionFacet[];
  selectedIds: string[];
  onToggle: (sessionId: string) => void;
}) {
  const [query, setQuery] = useState("");

  const sessionMap = useMemo(() => new Map(sessions.map((s) => [s.session_id, s])), [sessions]);

  const pinnedSessions = useMemo(
    () => selectedIds.map((id) => sessionMap.get(id)).filter(Boolean) as ImportSessionFacet[],
    [selectedIds, sessionMap],
  );

  const filteredSessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    return sessions
      .filter((s) => !selectedIds.includes(s.session_id))
      .filter(
        (s) => !q || s.file_key.toLowerCase().includes(q) || s.session_id.toLowerCase().includes(q),
      )
      .slice(0, SESSION_PAGE_SIZE);
  }, [sessions, selectedIds, query]);

  const totalUnselected = sessions.length - selectedIds.length;

  return (
    <FacetSection title="Import Sessions">
      {pinnedSessions.length > 0 && (
        <div className="flex flex-col gap-0.5 pb-1.5">
          {pinnedSessions.map((s) => (
            <SessionBadge key={s.session_id} session={s} onRemove={() => onToggle(s.session_id)} />
          ))}
        </div>
      )}

      {totalUnselected > 0 && (
        <div className="relative mb-1">
          <Search className="absolute left-1.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground pointer-events-none" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={`Search ${totalUnselected} sessions…`}
            className="w-full h-6 pl-5 pr-2 rounded border border-input bg-transparent text-[11px] text-foreground placeholder:text-muted-foreground outline-none focus:border-ring transition-colors"
          />
          {query && (
            <button
              onClick={() => setQuery("")}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3" />
            </button>
          )}
        </div>
      )}

      {filteredSessions.length > 0 && (
        <ScrollArea className="max-h-[240px]">
          <div className="flex flex-col gap-0.5 pr-2">
            {filteredSessions.map((s) => (
              <SessionRow
                key={s.session_id}
                session={s}
                onCheckedChange={() => onToggle(s.session_id)}
              />
            ))}
          </div>
        </ScrollArea>
      )}

      {filteredSessions.length === 0 && query && (
        <p className="text-[10px] text-muted-foreground px-1 py-1">No sessions match</p>
      )}
    </FacetSection>
  );
}

function SessionBadge({
  session,
  onRemove,
}: {
  session: ImportSessionFacet;
  onRemove: () => void;
}) {
  const name = basename(session.file_key) || session.session_id.slice(0, 8);
  return (
    <div
      className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-muted/60 group"
      title={session.file_key}
    >
      <span className="flex-1 min-w-0 font-mono text-[10px] text-foreground truncate">{name}</span>
      <span className="text-[10px] text-muted-foreground tabular-nums shrink-0">
        {session.count}
      </span>
      <button
        onClick={onRemove}
        className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
        aria-label="Remove filter"
      >
        <X className="size-2.5" />
      </button>
    </div>
  );
}

function SessionRow({
  session,
  onCheckedChange,
}: {
  session: ImportSessionFacet;
  onCheckedChange: () => void;
}) {
  const name = basename(session.file_key) || session.session_id.slice(0, 8);
  return (
    <label
      className="flex items-center gap-1.5 py-0.5 cursor-pointer group"
      title={session.file_key}
    >
      <Checkbox checked={false} onCheckedChange={onCheckedChange} className="size-3 shrink-0" />
      <span className="flex-1 min-w-0 font-mono text-[10px] text-foreground truncate leading-tight">
        {name}
      </span>
      <span className="text-[10px] text-muted-foreground tabular-nums shrink-0">
        {session.count}
      </span>
    </label>
  );
}

// ─── Shared primitives ────────────────────────────────────────────────────────

function FacetSection({
  title,
  defaultOpen = false,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Collapsible defaultOpen={defaultOpen}>
      <CollapsibleTrigger className="flex w-full items-center gap-1 py-1.5 text-[10px] font-semibold text-muted-foreground uppercase tracking-wider hover:text-foreground transition-colors group">
        <ChevronRight className="size-3 shrink-0 transition-transform group-data-[state=open]:rotate-90" />
        {title}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="flex flex-col gap-0.5 pb-2 pl-1">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function FacetRow({
  checked,
  onCheckedChange,
  label,
  count,
}: {
  checked: boolean;
  onCheckedChange: () => void;
  label: React.ReactNode;
  count: number;
}) {
  return (
    <label className="flex items-center gap-1.5 py-0.5 cursor-pointer text-[12px]">
      <Checkbox checked={checked} onCheckedChange={onCheckedChange} className="size-3 shrink-0" />
      <span className="flex-1 min-w-0 truncate">{label}</span>
      <span className="text-[10px] text-muted-foreground tabular-nums shrink-0">{count}</span>
    </label>
  );
}

// ─── Skeleton ─────────────────────────────────────────────────────────────────

function FacetSkeleton() {
  return (
    <div className="flex flex-col gap-3 animate-pulse">
      <SkeletonSection rows={5} />
      <SkeletonSection rows={2} />
      <SkeletonSection rows={3} />
    </div>
  );
}

function SkeletonSection({ rows }: { rows: number }) {
  return (
    <div className="flex flex-col gap-1.5 pl-1">
      <div className="h-2.5 w-16 rounded bg-muted" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-1.5">
          <div className="size-3 rounded-sm bg-muted shrink-0" />
          <div className="h-2.5 rounded bg-muted" style={{ width: `${55 + ((i * 17) % 30)}%` }} />
          <div className="h-2.5 w-6 rounded bg-muted ml-auto shrink-0" />
        </div>
      ))}
    </div>
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function basename(path: string): string {
  if (!path) return "";
  const slash = path.lastIndexOf("/");
  return slash >= 0 ? path.slice(slash + 1) : path;
}

import { useCallback } from "react";
import type { TMFacets } from "./types";
import { ENTITY_TYPES } from "./types";
import { Checkbox } from "../ui/checkbox";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "../ui/collapsible";
import { LocalePill } from "./LocalePill";
import { ChevronRight } from "lucide-react";
import { relativeTime } from "./utils";

export interface FacetSelection {
  locales: string[];
  projects: string[];
  entityTypes: string[];
  sessionIds: string[];
  codeFilter: "all" | "has_codes" | "no_codes";
}

const EMPTY_FACETS: FacetSelection = {
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

/**
 * Left sidebar showing faceted filters for the TM browser.
 * Each section is collapsible with checkboxes and counts.
 */
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
    <div className="flex flex-col gap-1 text-sm">
      <div className="flex items-center justify-between mb-1">
        <h3 className="text-[13px] font-semibold text-foreground">Filters</h3>
        {hasActiveFilters && (
          <button
            onClick={() => onSelectionChange(EMPTY_FACETS)}
            className="text-[10px] text-primary hover:text-primary/80"
          >
            Clear all
          </button>
        )}
      </div>

      {/* Languages */}
      {facets && facets.locales.length > 0 && (
        <FacetSection title="Languages" defaultOpen>
          {facets.locales.map(({ locale, count }) => (
            <FacetItem
              key={locale}
              checked={selection.locales.includes(locale)}
              onCheckedChange={() => toggleLocale(locale)}
              label={<LocalePill locale={locale} />}
              count={count}
            />
          ))}
        </FacetSection>
      )}

      {/* Projects */}
      {facets && facets.projects.length > 0 && (
        <FacetSection title="Project">
          {facets.projects.map((p) => (
            <FacetItem
              key={p.project_id || "__none__"}
              checked={selection.projects.includes(p.project_id)}
              onCheckedChange={() => toggleProject(p.project_id)}
              label={p.project_id || "No project"}
              count={p.count}
            />
          ))}
        </FacetSection>
      )}

      {/* Entity Types */}
      {facets && facets.entity_types.length > 0 && (
        <FacetSection title="Entity Types">
          {facets.entity_types.map((et) => {
            const label = ENTITY_TYPES.find((t) => t.value === et.type)?.label ?? et.type;
            return (
              <FacetItem
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

      {/* Import Sessions */}
      {facets && facets.import_sessions.length > 0 && (
        <FacetSection title="Import Sessions">
          {facets.import_sessions.map((s) => (
            <FacetItem
              key={s.session_id}
              checked={selection.sessionIds.includes(s.session_id)}
              onCheckedChange={() => toggleSession(s.session_id)}
              label={
                <span className="flex flex-col min-w-0">
                  <span className="truncate" title={s.file_key}>
                    {s.file_key || s.session_id}
                  </span>
                  <span className="text-[9px] text-muted-foreground">
                    {relativeTime(s.imported_at)}
                  </span>
                </span>
              }
              count={s.count}
            />
          ))}
        </FacetSection>
      )}

      {/* Inline Codes */}
      {facets && (facets.has_codes > 0 || facets.no_codes > 0) && (
        <FacetSection title="Inline Codes">
          <FacetItem
            checked={selection.codeFilter === "has_codes"}
            onCheckedChange={() => setCodeFilter("has_codes")}
            label="Has inline codes"
            count={facets.has_codes}
          />
          <FacetItem
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
      <CollapsibleTrigger className="flex w-full items-center gap-1 py-1.5 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider hover:text-foreground transition-colors group">
        <ChevronRight className="size-3 transition-transform group-data-[state=open]:rotate-90" />
        {title}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="flex flex-col gap-0.5 pb-2 pl-1">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function FacetItem({
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
      <Checkbox checked={checked} onCheckedChange={onCheckedChange} className="size-3.5" />
      <span className="flex-1 truncate">{label}</span>
      <span className="text-[10px] text-muted-foreground tabular-nums">{count}</span>
    </label>
  );
}

export { EMPTY_FACETS };

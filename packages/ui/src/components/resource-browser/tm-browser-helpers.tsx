import { t } from "@neokapi/kapi-react/runtime";
import type { Run } from "@neokapi/kapi-format";
import { flattenRuns } from "@neokapi/kapi-format";
import type {
  TMEntryDTO,
  TMFacets,
  TMSearchFilter,
  EntityAnnotationDTO,
  VariantDTO,
  VariantInputDTO,
} from "./types";
import type { FilterToken, FilterField } from "./TMSearchBar";
import type { FacetSelection } from "./TMFacetSidebar";

export type ViewMode = "bilingual" | "multilang";

export const PAGE_SIZE = 50;

/**
 * Builds a variant input from a Run sequence, deriving the flattened
 * plain text and attaching the runs when any inline content is present.
 */
export function variantForInput(runs: Run[]): VariantInputDTO {
  const input: VariantInputDTO = { text: flattenRuns(runs) };
  if (runs.length > 0) input.runs = runs;
  return input;
}

/**
 * Returns a TMEntryDTO where hint_src_lang is overridden for display
 * purposes when the caller (bilingual view) has picked a specific source.
 * When `override` is null or not present as a variant, the original entry
 * is returned untouched.
 */
export function withHint(entry: TMEntryDTO, override: string | null): TMEntryDTO {
  if (!override) return entry;
  const variant: VariantDTO | undefined = entry.variants[override];
  if (!variant) return entry;
  return { ...entry, hint_src_lang: override };
}

/** Maps the all/some selection pair onto the tri-state Checkbox value. */
export function triStateChecked(
  allSelected: boolean,
  someSelected: boolean,
): boolean | "indeterminate" {
  if (allSelected) return true;
  if (someSelected) return "indeterminate";
  return false;
}

/** Builds the search filter from facet selection + tokens + marked entities. */
export function buildSearchFilter(
  facetSelection: FacetSelection,
  filterTokens: FilterToken[],
  markedEntities: EntityAnnotationDTO[],
): TMSearchFilter {
  const filter: TMSearchFilter = {};
  const tokenProject = filterTokens.find((t) => t.key === "project")?.value;
  if (tokenProject) filter.project_id = tokenProject;
  else if (facetSelection.projects.length === 1) filter.project_id = facetSelection.projects[0];
  if (facetSelection.entityTypes.length > 0) filter.entity_types = facetSelection.entityTypes;
  if (facetSelection.sessionIds.length > 0) filter.session_ids = facetSelection.sessionIds;
  if (facetSelection.codeFilter === "has_codes") filter.has_codes = true;
  if (facetSelection.codeFilter === "no_codes") filter.has_codes = false;
  if (markedEntities.length > 0) {
    filter.entity_values = markedEntities.map((e) => ({ value: e.text, type: e.type }));
  }
  return filter;
}

/** Builds filter fields from facet data for the search bar's filter dropdown. */
export function buildSearchBarFilterFields(facets: TMFacets | null): FilterField[] {
  if (!facets) return [];
  const fields: FilterField[] = [];
  if (facets.locales.length > 0) {
    fields.push({
      key: "language",
      label: t("Language"),
      values: facets.locales.map((l) => ({ value: l.locale, label: l.locale })),
    });
  }
  if (facets.projects.length > 0) {
    fields.push({
      key: "project",
      label: t("Project"),
      values: facets.projects.map((p) => ({
        value: p.project_id,
        label: p.project_id || t("No project"),
      })),
    });
  }
  return fields;
}

// The filters of the Content memory list: the store's facets offered as the
// fields of the filter bar, and the tokens a person picks turned back into the
// search the store answers.
//
// Each facet the store counts is one field. `language` narrows the search to
// wording in that language, `target` keeps the pairs into that language,
// `project`, `entity` and `import` narrow by the entry's project, the kinds of
// name it holds and the import it came from, and `codes` keeps the entries
// with or without inline codes.

import { t } from "@neokapi/i18n-react/runtime";
import type {
  FilterField,
  FilterToken,
  MemoryFacets,
  MemorySearchFilter,
} from "@neokapi/ui-primitives";

/** The filter-bar keys, in the order the Filters menu lists them. */
export const MEMORY_FILTER_KEYS = [
  "language",
  "target",
  "project",
  "entity",
  "import",
  "codes",
] as const;

/** What the store is asked for, from the filter tokens. */
export interface MemoryQueryParts {
  /** Search within wording in this language; empty searches every language. */
  anyLocale: string;
  /** Keep the entries holding this language, and only their pairs into it. */
  requireLocale: string;
  filter: MemorySearchFilter;
}

/** The fields the filter bar offers, from the facets the store counts. */
export function memoryFilterFields(facets: MemoryFacets | null | undefined): FilterField[] {
  if (!facets) return [];
  const fields: FilterField[] = [];
  const locales = facets.locales.map((l) => ({
    value: l.locale,
    label: `${l.locale} · ${l.count}`,
  }));
  if (locales.length > 0) {
    fields.push({
      key: "language",
      label: t("Language"),
      hint: t("search the wording in one language"),
      values: locales,
    });
    fields.push({
      key: "target",
      label: t("Target language"),
      hint: t("show the pairs into one language"),
      values: locales,
    });
  }
  if (facets.projects.length > 0) {
    fields.push({
      key: "project",
      label: t("Project"),
      hint: t("entries held by one project"),
      values: facets.projects.map((p) => ({
        value: p.project_id,
        label: `${p.project_id || t("No project")} · ${p.count}`,
      })),
    });
  }
  if (facets.entity_types.length > 0) {
    fields.push({
      key: "entity",
      label: t("Entity"),
      hint: t("entries that hold a kind of name"),
      values: facets.entity_types.map((e) => ({ value: e.type, label: `${e.type} · ${e.count}` })),
    });
  }
  if (facets.import_sessions.length > 0) {
    fields.push({
      key: "import",
      label: t("Import"),
      hint: t("entries one import brought in"),
      values: facets.import_sessions.map((s) => ({
        value: s.session_id,
        label: `${s.file_key} · ${s.count}`,
      })),
    });
  }
  if (facets.has_codes > 0 || facets.no_codes > 0) {
    fields.push({
      key: "codes",
      label: t("Inline codes"),
      hint: t("entries with or without inline codes"),
      values: [
        { value: "with", label: `${t("With inline codes")} · ${facets.has_codes}` },
        { value: "without", label: `${t("Without inline codes")} · ${facets.no_codes}` },
      ],
    });
  }
  return fields;
}

/** The search the store answers, from the tokens in the filter bar. */
export function memoryQueryParts(
  tokens: FilterToken[],
  targetLocales?: string[],
): MemoryQueryParts {
  const first = (key: string) => tokens.find((tok) => tok.key === key)?.value ?? "";
  const all = (key: string) => tokens.filter((tok) => tok.key === key).map((tok) => tok.value);
  const filter: MemorySearchFilter = {};
  const project = first("project");
  if (tokens.some((tok) => tok.key === "project")) filter.project_id = project;
  const entities = all("entity");
  if (entities.length > 0) filter.entity_types = entities;
  const sessions = all("import");
  if (sessions.length > 0) filter.session_ids = sessions;
  const codes = first("codes");
  if (codes === "with") filter.has_codes = true;
  if (codes === "without") filter.has_codes = false;
  // A target picked in the bar wins over the Active Filter's one language.
  const target = first("target") || (targetLocales?.length === 1 ? targetLocales[0] : "");
  return { anyLocale: first("language"), requireLocale: target, filter };
}

/** The target languages whose pairs the list shows: the bar's, else the Active Filter's. */
export function memoryTargets(
  tokens: FilterToken[],
  targetLocales?: string[],
): ReadonlySet<string> | undefined {
  const picked = tokens.filter((tok) => tok.key === "target").map((tok) => tok.value);
  if (picked.length > 0) return new Set(picked);
  return targetLocales?.length ? new Set(targetLocales) : undefined;
}

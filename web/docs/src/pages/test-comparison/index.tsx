import { useState, useMemo, useCallback } from "react";
import Layout from "@theme/Layout";
import type { TestComparisonData, FilterComparison, StateFilter } from "./_types";
import { normalizeFilter, normalizeSummary } from "./_types";
import SummaryBar from "./_SummaryBar";
import FilterCard, { FilterColumnHeadings } from "./_FilterCard";
import styles from "./_index.module.css";
import comparisonData from "@site/static/data/contract-audit.json";

type Side = "okapi" | "bridge" | "native";

const sideLabels: Record<Side, string> = {
  okapi: "Okapi",
  bridge: "Bridge",
  native: "Native",
};

const raw = comparisonData as unknown as TestComparisonData;

/** Check if a filter has any test cases matching a state. */
function filterHasState(f: FilterComparison, state: StateFilter): boolean {
  if (!state) return true;
  return f.testCases.some((tc) => {
    switch (state) {
      case "implemented":
        return tc.testState === "implemented";
      case "not-applicable":
        return tc.testState !== "implemented" && tc.testState !== "pending" && !!tc.skipReason;
      case "pending":
        return tc.testState === "pending";
      case "unmapped":
        return !tc.testState && !tc.skipReason;
      default:
        return true;
    }
  });
}

export default function TestComparison() {
  const [search, setSearch] = useState("");
  const [activeSides, setActiveSides] = useState<Set<Side>>(new Set());
  const [stateFilter, setStateFilter] = useState<StateFilter>(null);

  const allSelected = activeSides.size === 0;

  const toggleSide = useCallback((side: Side) => {
    setActiveSides((prev) => {
      const next = new Set(prev);
      if (next.has(side)) {
        next.delete(side);
      } else {
        next.add(side);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setActiveSides(new Set());
    setStateFilter(null);
  }, []);

  const handleStateFilter = useCallback((state: StateFilter) => {
    setStateFilter((prev) => (prev === state ? null : state));
  }, []);

  const data = useMemo(
    () => ({
      ...raw,
      summary: normalizeSummary(raw.summary),
      filters: raw.filters.map(normalizeFilter),
    }),
    [],
  );

  const filtered = data.filters.filter((f: FilterComparison) => {
    if (search) {
      const q = search.toLowerCase();
      const matchesName = f.filterName.toLowerCase().includes(q);
      const matchesNative = f.nativeFilterName?.toLowerCase().includes(q) ?? false;
      if (!matchesName && !matchesNative) return false;
    }
    if (!allSelected) {
      if (activeSides.has("okapi") && f.okapi == null) return false;
      if (activeSides.has("bridge") && f.bridge == null) return false;
      if (activeSides.has("native") && f.native == null) return false;
    }
    if (stateFilter && !filterHasState(f, stateFilter)) return false;
    return true;
  });

  // Subfilters are filters the bridge only invokes through a parent
  // (e.g. ICU MessageFormat inside a Properties value). Render them in
  // their own section so they don't dilute top-level coverage stats and
  // so reviewers see why they have no bridge column.
  const topLevelFilters = filtered.filter((f) => f.specKind !== "subfilter");
  const subfilterFilters = filtered.filter((f) => f.specKind === "subfilter");

  const cardFor = (fc: FilterComparison) => (
    <FilterCard
      key={fc.filterName}
      filter={fc}
      goCommitSHA={data.goCommitSHA}
      okapiTag={data.okapiTag}
      defaultExpanded={stateFilter !== null}
      defaultTestFilter={
        stateFilter === "not-applicable"
          ? "not-applicable"
          : stateFilter === "unmapped"
            ? "unmapped"
            : stateFilter === "pending"
              ? "pending"
              : stateFilter === "implemented"
                ? "implemented"
                : undefined
      }
    />
  );

  return (
    <Layout title="Test Comparison" description="Okapi vs neokapi filter test comparison">
      <main className="container margin-vert--lg">
        <h1>Filter Test Comparison</h1>
        <p>
          Side-by-side view of Okapi Framework (Java) tests and their neokapi bridge and native
          counterparts.
        </p>

        <SummaryBar
          summary={data.summary}
          generatedAt={data.generatedAt}
          filters={data.filters}
          stateFilter={stateFilter}
          onStateFilter={handleStateFilter}
        />

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.searchInput}
            placeholder="Search filters..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className={styles.filterButtons}>
            <button
              className={`button button--sm ${allSelected && !stateFilter ? "button--primary" : "button--outline button--secondary"}`}
              onClick={selectAll}
            >
              All
            </button>
            {(["okapi", "bridge", "native"] as Side[]).map((side) => (
              <button
                key={side}
                className={`button button--sm ${activeSides.has(side) ? "button--primary" : "button--outline button--secondary"}`}
                onClick={() => toggleSide(side)}
              >
                {sideLabels[side]}
              </button>
            ))}
          </div>
        </div>

        {stateFilter && (
          <div className={styles.activeFilterBanner}>
            Showing formats with{" "}
            <strong>{stateFilter === "not-applicable" ? "not applicable" : stateFilter}</strong>{" "}
            tests ({filtered.length} formats)
            <button
              className="button button--sm button--outline button--secondary"
              style={{ marginLeft: "0.75rem" }}
              onClick={() => setStateFilter(null)}
            >
              Clear
            </button>
          </div>
        )}

        <div className={styles.filterList}>
          <FilterColumnHeadings />
          {topLevelFilters.map(cardFor)}
          {filtered.length === 0 && <p>No filters match your search.</p>}
        </div>

        {subfilterFilters.length > 0 && (
          <>
            <h2 className={styles.subfilterHeading}>Layer formats (subfilters)</h2>
            <p className={styles.subfilterNote}>
              These filters are invoked by a parent filter when it encounters embedded content (e.g.
              ICU MessageFormat inside a Properties value). They have no top-level bridge schema and
              are not dispatched standalone, so the bridge column and parity runner show no data —
              the native runner still verifies the spec.
            </p>
            <div className={styles.filterList}>
              <FilterColumnHeadings />
              {subfilterFilters.map(cardFor)}
            </div>
          </>
        )}
      </main>
    </Layout>
  );
}

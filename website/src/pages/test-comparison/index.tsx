import {useState, useMemo, useCallback} from 'react';
import Layout from '@theme/Layout';
import type {TestComparisonData, FilterComparison} from './_types';
import {normalizeFilter, normalizeSummary} from './_types';
import SummaryBar from './_SummaryBar';
import FilterCard, {FilterColumnHeadings} from './_FilterCard';
import styles from './_index.module.css';
import comparisonData from '@site/static/data/test-comparison.json';

type Side = 'okapi' | 'bridge' | 'native';

const sideLabels: Record<Side, string> = {
  okapi: 'Okapi',
  bridge: 'Bridge',
  native: 'Native',
};

const raw = comparisonData as unknown as TestComparisonData;

export default function TestComparison() {
  const [search, setSearch] = useState('');
  const [activeSides, setActiveSides] = useState<Set<Side>>(new Set());

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
    if (search && !f.filterName.toLowerCase().includes(search.toLowerCase()))
      return false;
    if (allSelected) return true;
    if (activeSides.has('okapi') && f.okapi == null) return false;
    if (activeSides.has('bridge') && f.bridge == null) return false;
    if (activeSides.has('native') && f.native == null) return false;
    return true;
  });

  return (
    <Layout
      title="Test Comparison"
      description="Okapi vs gokapi filter test comparison">
      <main className="container margin-vert--lg">
        <h1>Filter Test Comparison</h1>
        <p>
          Side-by-side view of Okapi Framework (Java) tests and their gokapi
          bridge and native counterparts.
        </p>

        <SummaryBar
          summary={data.summary}
          generatedAt={data.generatedAt}
          filters={data.filters}
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
              className={`button button--sm ${allSelected ? 'button--primary' : 'button--outline button--secondary'}`}
              onClick={selectAll}>
              All
            </button>
            {(['okapi', 'bridge', 'native'] as Side[]).map((side) => (
              <button
                key={side}
                className={`button button--sm ${activeSides.has(side) ? 'button--primary' : 'button--outline button--secondary'}`}
                onClick={() => toggleSide(side)}>
                {sideLabels[side]}
              </button>
            ))}
          </div>
        </div>

        <div className={styles.filterList}>
          <FilterColumnHeadings />
          {filtered.map((fc: FilterComparison) => (
            <FilterCard
              key={fc.filterName}
              filter={fc}
              goCommitSHA={data.goCommitSHA}
              okapiTag={data.okapiTag}
            />
          ))}
          {filtered.length === 0 && <p>No filters match your search.</p>}
        </div>
      </main>
    </Layout>
  );
}

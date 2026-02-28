import {useState, useMemo} from 'react';
import Layout from '@theme/Layout';
import type {TestComparisonData, FilterComparison} from './_types';
import {normalizeFilter, normalizeSummary} from './_types';
import SummaryBar from './_SummaryBar';
import FilterCard from './_FilterCard';
import styles from './_index.module.css';
import comparisonData from '@site/static/data/test-comparison.json';

type FilterMode =
  | 'all'
  | 'both'
  | 'okapi-only'
  | 'bridge-only'
  | 'native-only';

const filterLabels: Record<FilterMode, string> = {
  all: 'All',
  both: 'Both sides',
  'okapi-only': 'Okapi only',
  'bridge-only': 'Bridge only',
  'native-only': 'Native only',
};

const raw = comparisonData as TestComparisonData;

export default function TestComparison() {
  const [search, setSearch] = useState('');
  const [filterMode, setFilterMode] = useState<FilterMode>('all');

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
    switch (filterMode) {
      case 'both':
        return f.okapi != null && (f.bridge != null || f.native != null);
      case 'okapi-only':
        return f.okapi != null && f.bridge == null && f.native == null;
      case 'bridge-only':
        return f.bridge != null;
      case 'native-only':
        return f.native != null;
      default:
        return true;
    }
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

        <SummaryBar summary={data.summary} generatedAt={data.generatedAt} />

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.searchInput}
            placeholder="Search filters..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className={styles.filterButtons}>
            {(
              [
                'all',
                'both',
                'okapi-only',
                'bridge-only',
                'native-only',
              ] as FilterMode[]
            ).map((m) => (
              <button
                key={m}
                className={`button button--sm ${filterMode === m ? 'button--primary' : 'button--outline button--secondary'}`}
                onClick={() => setFilterMode(m)}>
                {filterLabels[m]}
              </button>
            ))}
          </div>
        </div>

        <div className={styles.filterList}>
          {filtered.map((fc: FilterComparison) => (
            <FilterCard key={fc.filterName} filter={fc} />
          ))}
          {filtered.length === 0 && <p>No filters match your search.</p>}
        </div>
      </main>
    </Layout>
  );
}

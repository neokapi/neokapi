import {useState} from 'react';
import Layout from '@theme/Layout';
import type {TestComparisonData} from './_types';
import SummaryBar from './_SummaryBar';
import FilterCard from './_FilterCard';
import styles from './_index.module.css';
import comparisonData from '@site/static/data/test-comparison.json';

type FilterMode = 'all' | 'both' | 'okapi-only' | 'gokapi-only';

const filterLabels: Record<FilterMode, string> = {
  all: 'All',
  both: 'Both sides',
  'okapi-only': 'Okapi only',
  'gokapi-only': 'Gokapi only',
};

const data = comparisonData as TestComparisonData;

export default function TestComparison() {
  const [search, setSearch] = useState('');
  const [filterMode, setFilterMode] = useState<FilterMode>('all');

  const filtered = data.filters.filter((f) => {
    if (search && !f.filterName.toLowerCase().includes(search.toLowerCase()))
      return false;
    switch (filterMode) {
      case 'both':
        return f.okapi != null && f.gokapi != null;
      case 'okapi-only':
        return f.okapi != null && f.gokapi == null;
      case 'gokapi-only':
        return f.okapi == null && f.gokapi != null;
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
          Side-by-side view of Okapi Framework (Java) and gokapi bridge filter
          tests.
        </p>

        <SummaryBar
          summary={data.summary}
          generatedAt={data.generatedAt}
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
            {(
              ['all', 'both', 'okapi-only', 'gokapi-only'] as FilterMode[]
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
          {filtered.map((fc) => (
            <FilterCard key={fc.filterName} filter={fc} />
          ))}
          {filtered.length === 0 && <p>No filters match your search.</p>}
        </div>
      </main>
    </Layout>
  );
}

import {useState, useMemo} from 'react';
import Layout from '@theme/Layout';
import type {FormatsData} from './_types';
import FormatCard from './_FormatCard';
import styles from './_index.module.css';
import formatsData from '@site/static/data/formats.json';

const data = formatsData as unknown as FormatsData;

export default function Formats() {
  const [search, setSearch] = useState('');

  const filtered = useMemo(() => {
    if (!search) return data.formats;
    const q = search.toLowerCase();
    return data.formats.filter(
      (f) =>
        f.displayName.toLowerCase().includes(q) ||
        f.id.toLowerCase().includes(q) ||
        f.extensions?.some((e) => e.toLowerCase().includes(q)) ||
        f.mimeTypes?.some((m) => m.toLowerCase().includes(q)),
    );
  }, [search]);

  const configurable = data.formats.filter(
    (f) => f.properties && Object.keys(f.properties).length > 0,
  );
  const totalParams = data.formats.reduce(
    (sum, f) => sum + Object.keys(f.properties ?? {}).length,
    0,
  );

  return (
    <Layout
      title="Format Reference"
      description="Interactive reference for all neokapi built-in formats with configurable parameters">
      <main className="container margin-vert--lg">
        <h1>Format Reference</h1>
        <p>
          Interactive documentation for all {data.formats.length} built-in
          formats. {configurable.length} formats have configurable parameters
          ({totalParams} total). Edit parameter values below to generate
          configuration output.
        </p>

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.searchInput}
            placeholder="Search formats by name, extension, or MIME type..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <span className={styles.formatCount}>
            {filtered.length} of {data.formats.length} formats
          </span>
        </div>

        <div className={styles.formatList}>
          {filtered.map((format) => (
            <FormatCard key={format.id} format={format} />
          ))}
        </div>
      </main>
    </Layout>
  );
}

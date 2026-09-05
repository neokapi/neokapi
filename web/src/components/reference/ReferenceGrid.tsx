import { useState, useMemo, useCallback } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import ReferenceCard from "./ReferenceCard";
import { categoryLabel, SOURCE_LABELS, sourceLabel } from "./labels";
import { builtinToolIds, formatHref, toolHref } from "./slugs";
import styles from "./styles.module.css";

type Filter = "all" | "built-in" | "plugin";

// The filter buttons: the sources, and all of them. Held at module level so
// the build-time transform rewrites each call: inside a text-bearing element
// it would not.
const FILTER_LABELS: Record<Filter, string> = {
  all: t("All", "source filter"),
  "built-in": SOURCE_LABELS["built-in"],
  plugin: SOURCE_LABELS.plugin,
};

const SEARCH_PLACEHOLDER = {
  format: t("Search by name, extension, or MIME type…", "search box placeholder"),
  tool: t("Search by name, category, or tag…", "search box placeholder"),
};

const NOUN = {
  format: t("formats", "plural noun in a result count"),
  tool: t("tools", "plural noun in a result count"),
};

function headingLabel(kind: "format" | "tool", key: string): string {
  return kind === "tool" ? categoryLabel(key) : sourceLabel(key);
}

interface Props {
  entries: ReferenceEntry[];
  /** "format" | "tool" — controls placeholder copy and category grouping. */
  kind: "format" | "tool";
}

function matches(entry: ReferenceEntry, q: string): boolean {
  if (entry.displayName.toLowerCase().includes(q)) return true;
  if (entry.id.toLowerCase().includes(q)) return true;
  if (entry.extensions?.some((e) => e.toLowerCase().includes(q))) return true;
  if (entry.mimeTypes?.some((m) => m.toLowerCase().includes(q))) return true;
  if (entry.category?.toLowerCase().includes(q)) return true;
  if (entry.tags?.some((t) => t.toLowerCase().includes(q))) return true;
  return false;
}

export default function ReferenceGrid({ entries, kind }: Props) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const counts = useMemo(() => {
    const by = (s: ReferenceSource) => entries.filter((e) => e.source === s).length;
    return {
      all: entries.length,
      "built-in": by("built-in"),
      plugin: by("plugin"),
    };
  }, [entries]);

  // Tool slugs need the built-in id set to disambiguate cross-source collisions
  // (a built-in and a plugin tool can share an id). Formats have unique ids.
  const builtins = useMemo(() => builtinToolIds(entries), [entries]);
  const hrefFor = useCallback(
    (entry: ReferenceEntry) => (kind === "format" ? formatHref(entry) : toolHref(entry, builtins)),
    [kind, builtins],
  );

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return entries.filter((e) => {
      if (filter !== "all" && e.source !== filter) return false;
      if (q && !matches(e, q)) return false;
      return true;
    });
  }, [entries, search, filter]);

  // Tools group by category. Formats split by source (native engine vs
  // plugin) so the two surfaces read as distinct sections — but only while the
  // "All" filter is active; once a single source is selected the split is moot,
  // so the grid goes flat. Within each format section, the alphabetical sort
  // from the caller is preserved.
  const grouped = useMemo(() => {
    if (kind === "tool") {
      const map = new Map<string, ReferenceEntry[]>();
      for (const e of filtered) {
        const cat = e.category || "other";
        const list = map.get(cat) ?? [];
        list.push(e);
        map.set(cat, list);
      }
      return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
    }
    // Formats, "All" filter: section by source, built-in first.
    if (filter === "all") {
      const builtin = filtered.filter((e) => e.source === "built-in");
      const plugin = filtered.filter((e) => e.source === "plugin");
      const sections: [string, ReferenceEntry[]][] = [];
      if (builtin.length) sections.push(["built-in", builtin]);
      if (plugin.length) sections.push(["plugin", plugin]);
      return sections;
    }
    return null;
  }, [filtered, kind, filter]);

  const filterButton = (value: Filter) => (
    <button
      type="button"
      className={`${styles.filterButton} ${filter === value ? styles.filterButtonActive : ""}`}
      onClick={() => setFilter(value)}
      aria-pressed={filter === value}
    >
      {FILTER_LABELS[value]}
      <span className={styles.filterCount}>{counts[value]}</span>
    </button>
  );

  const noun = NOUN[kind];

  return (
    <>
      <div className={styles.toolbar}>
        <input
          type="text"
          className={styles.search}
          placeholder={SEARCH_PLACEHOLDER[kind]}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {counts.plugin > 0 && (
          <div className={styles.filterGroup} role="group" aria-label="Filter by source">
            {filterButton("all")}
            {filterButton("built-in")}
            {filterButton("plugin")}
          </div>
        )}
      </div>

      <p className={styles.resultCount}>
        {filtered.length} of {entries.length} {noun}
      </p>

      {grouped ? (
        grouped.map(([cat, items]) => (
          <section key={cat} className={styles.categorySection}>
            <h2
              className={`${styles.categoryHeading} ${
                kind === "format" ? styles.sourceHeading : ""
              }`}
            >
              {headingLabel(kind, cat)}
              <span className={styles.categoryCount}>{items.length}</span>
            </h2>
            <div className={styles.grid}>
              {items.map((entry) => (
                <ReferenceCard key={entry.id} entry={entry} href={hrefFor(entry)} />
              ))}
            </div>
          </section>
        ))
      ) : (
        <div className={styles.grid}>
          {filtered.map((entry) => (
            <ReferenceCard key={entry.id} entry={entry} href={hrefFor(entry)} />
          ))}
        </div>
      )}

      {filtered.length === 0 && <p className={styles.empty}>No {noun} match your search.</p>}
    </>
  );
}

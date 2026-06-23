import { useState, useMemo, useCallback } from "react";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import ReferenceCard from "./ReferenceCard";
import { builtinToolIds, formatHref, toolHref } from "./slugs";
import styles from "./styles.module.css";

type Filter = "all" | ReferenceSource;

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
      okapi: by("okapi"),
    };
  }, [entries]);

  // Tool slugs need the built-in id set to disambiguate cross-source collisions
  // (a built-in and an Okapi tool can share an id). Formats have unique ids.
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

  // Tools group by category. Formats split by source (native engine vs Okapi
  // bridge) so the two surfaces read as distinct sections — but only while the
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
      const okapi = filtered.filter((e) => e.source === "okapi");
      const sections: [string, ReferenceEntry[]][] = [];
      if (builtin.length) sections.push(["Built-in", builtin]);
      if (plugin.length) sections.push(["Plugin", plugin]);
      if (okapi.length) sections.push(["Okapi bridge", okapi]);
      return sections;
    }
    return null;
  }, [filtered, kind, filter]);

  const filterButton = (value: Filter, label: string) => (
    <button
      type="button"
      className={`${styles.filterButton} ${filter === value ? styles.filterButtonActive : ""}`}
      onClick={() => setFilter(value)}
      aria-pressed={filter === value}
    >
      {label}
      <span className={styles.filterCount}>{counts[value]}</span>
    </button>
  );

  return (
    <>
      <div className={styles.toolbar}>
        <input
          type="text"
          className={styles.search}
          placeholder={
            kind === "format"
              ? "Search by name, extension, or MIME type…"
              : "Search by name, category, or tag…"
          }
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className={styles.filterGroup} role="group" aria-label="Filter by source">
          {filterButton("all", "All")}
          {filterButton("built-in", "Built-in")}
          {counts.plugin > 0 && filterButton("plugin", "Plugin")}
          {filterButton("okapi", "Okapi bridge")}
        </div>
      </div>

      <p className={styles.resultCount}>
        {filtered.length} of {entries.length} {kind === "format" ? "formats" : "tools"}
      </p>

      {grouped ? (
        grouped.map(([cat, items]) => (
          <section key={cat} className={styles.categorySection}>
            <h2
              className={`${styles.categoryHeading} ${
                kind === "format" ? styles.sourceHeading : ""
              }`}
            >
              {cat}
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

      {filtered.length === 0 && (
        <p className={styles.empty}>
          No {kind === "format" ? "formats" : "tools"} match your search.
        </p>
      )}
    </>
  );
}

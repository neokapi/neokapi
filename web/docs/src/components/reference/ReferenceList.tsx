import { useState, useMemo } from "react";
import type { ReferenceEntry, ReferenceSource } from "@neokapi/reference-data";
import ReferenceCard from "./ReferenceCard";
import styles from "./styles.module.css";

type Filter = "all" | ReferenceSource;

interface Props {
  entries: ReferenceEntry[];
  /** "format" | "tool" — controls placeholder copy and grouping. */
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

export default function ReferenceList({ entries, kind }: Props) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const counts = useMemo(() => {
    const builtin = entries.filter((e) => e.source === "built-in").length;
    const okapi = entries.filter((e) => e.source === "okapi").length;
    return { all: entries.length, "built-in": builtin, okapi };
  }, [entries]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return entries.filter((e) => {
      if (filter !== "all" && e.source !== filter) return false;
      if (q && !matches(e, q)) return false;
      return true;
    });
  }, [entries, search, filter]);

  // Tools group by category; formats stay in a flat (already-sorted) list.
  const grouped = useMemo(() => {
    if (kind !== "tool") return null;
    const map = new Map<string, ReferenceEntry[]>();
    for (const e of filtered) {
      const cat = e.category || "other";
      const list = map.get(cat) ?? [];
      list.push(e);
      map.set(cat, list);
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [filtered, kind]);

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
          {filterButton("okapi", "Okapi bridge")}
        </div>
      </div>

      <p className={styles.resultCount}>
        {filtered.length} of {entries.length} {kind === "format" ? "formats" : "tools"}
      </p>

      {grouped ? (
        grouped.map(([cat, items]) => (
          <section key={cat} className={styles.categorySection}>
            <h2 className={styles.categoryHeading}>{cat}</h2>
            <div className={styles.list}>
              {items.map((entry) => (
                <ReferenceCard key={entry.id} entry={entry} />
              ))}
            </div>
          </section>
        ))
      ) : (
        <div className={styles.list}>
          {filtered.map((entry) => (
            <ReferenceCard key={entry.id} entry={entry} />
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

import { useState, useMemo, useEffect, useRef, useCallback } from "react";
import { useHistory, useLocation } from "@docusaurus/router";
import type { CommandEntry } from "@neokapi/reference-data";
import CommandCard from "./CommandCard";
import CommandModal from "./CommandModal";
import { commandName, commandSummary } from "./commandHelpers";
import styles from "./styles.module.css";

type Filter = "all" | "runnable" | "demo" | "network";

interface Props {
  commands: CommandEntry[];
}

// Display labels for the cobra group IDs (and a catch-all). Commands without a
// groupID fall into "Other".
const GROUP_LABELS: Record<string, string> = {
  processing: "Processing",
  translation: "Translation",
  quality: "Quality",
  analysis: "Analysis",
  "text-processing": "Text processing",
  content: "Content",
  management: "Management",
};

function groupLabel(groupID: string | undefined): string {
  if (!groupID) return "Other";
  return GROUP_LABELS[groupID] ?? groupID;
}

function matches(cmd: CommandEntry, q: string): boolean {
  if (commandName(cmd).toLowerCase().includes(q)) return true;
  if (cmd.id.toLowerCase().includes(q)) return true;
  if (commandSummary(cmd).toLowerCase().includes(q)) return true;
  if (cmd.aliases?.some((a) => a.toLowerCase().includes(q))) return true;
  if (cmd.flags?.some((f) => f.name.toLowerCase().includes(q))) return true;
  return false;
}

export default function CommandGrid({ commands }: Props) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const history = useHistory();
  const location = useLocation();

  // The selected command lives in the URL (?id=) so it's deep-linkable. Gate on
  // a client-mount flag: the static HTML carries no query, so reading it only
  // after mount keeps hydration in sync.
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const selectedId = mounted ? new URLSearchParams(location.search).get("id") : null;
  const selectedCmd = useMemo(
    () => (selectedId ? commands.find((c) => c.id === selectedId) : undefined),
    [commands, selectedId],
  );

  // Track whether we pushed a history entry so closing can pop it (preserving
  // the back button) rather than stacking a second one.
  const pushedRef = useRef(false);

  const select = useCallback(
    (id: string) => {
      const params = new URLSearchParams(location.search);
      params.set("id", id);
      history.push({ search: params.toString() });
      pushedRef.current = true;
    },
    [history, location.search],
  );

  const close = useCallback(() => {
    if (pushedRef.current) {
      pushedRef.current = false;
      history.goBack();
      return;
    }
    const params = new URLSearchParams(location.search);
    params.delete("id");
    history.replace({ search: params.toString() });
  }, [history, location.search]);

  const counts = useMemo(() => {
    const runnable = commands.filter((c) => c.runnableInBrowser && !c.demoMode).length;
    const demo = commands.filter((c) => c.runnableInBrowser && c.demoMode).length;
    const network = commands.filter((c) => !c.runnableInBrowser).length;
    return { all: commands.length, runnable, demo, network };
  }, [commands]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return commands.filter((c) => {
      if (filter === "runnable" && !(c.runnableInBrowser && !c.demoMode)) return false;
      if (filter === "demo" && !(c.runnableInBrowser && c.demoMode)) return false;
      if (filter === "network" && c.runnableInBrowser) return false;
      if (q && !matches(c, q)) return false;
      return true;
    });
  }, [commands, search, filter]);

  // Group by cobra group ID, then sort groups by label with "Other" last.
  const grouped = useMemo(() => {
    const map = new Map<string, CommandEntry[]>();
    for (const c of filtered) {
      const key = groupLabel(c.groupID);
      const list = map.get(key) ?? [];
      list.push(c);
      map.set(key, list);
    }
    return [...map.entries()].sort(([a], [b]) => {
      if (a === "Other") return 1;
      if (b === "Other") return -1;
      return a.localeCompare(b);
    });
  }, [filtered]);

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
          placeholder="Search by name, alias, flag, or description…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className={styles.filterGroup} role="group" aria-label="Filter by runnability">
          {filterButton("all", "All")}
          {filterButton("runnable", "Run")}
          {filterButton("demo", "Demo")}
          {filterButton("network", "Needs network")}
        </div>
      </div>

      <p className={styles.resultCount}>
        {filtered.length} of {commands.length} commands
      </p>

      {grouped.map(([group, items]) => (
        <section key={group} className={styles.groupSection}>
          <h2 className={styles.groupHeading}>{group}</h2>
          <div className={styles.grid}>
            {items.map((cmd) => (
              <CommandCard key={cmd.id} cmd={cmd} onSelect={select} />
            ))}
          </div>
        </section>
      ))}

      {filtered.length === 0 && <p className={styles.empty}>No commands match your search.</p>}

      {selectedCmd && <CommandModal cmd={selectedCmd} onClose={close} />}
    </>
  );
}

import { useState, useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { CommandEntry } from "@neokapi/reference-data";
import CommandCard from "./CommandCard";
import { commandName, commandSummary } from "./commandHelpers";
import { commandHref } from "@site/src/components/reference/slugs";
import styles from "./styles.module.css";

type Filter = "all" | "runnable" | "demo" | "network";

interface Props {
  commands: CommandEntry[];
}

// Labels for the cobra group ids the CLI declares (cli/app.go), and a
// catch-all for commands without one. Held at module level so the build-time
// transform rewrites each call: inside a text-bearing element it would not.
const OTHER_GROUP = "other";
const GROUP_LABELS: Record<string, string> = {
  work: t("Work", "command group heading"),
  languages: t("Languages", "command group heading"),
  assets: t("Assets", "command group heading"),
  advanced: t("Advanced", "command group heading"),
  [OTHER_GROUP]: t("Other", "command group heading"),
};

// The order the groups render in, the order cli.AddCommandGroups registers
// them for the root help: the writing commands (Work, Assets), then Languages,
// then Advanced. A group not listed here follows them, and the catch-all comes
// last.
const GROUP_ORDER = ["work", "assets", "languages", "advanced"];

function groupRank(key: string): number {
  if (key === OTHER_GROUP) return GROUP_ORDER.length + 1;
  const i = GROUP_ORDER.indexOf(key);
  return i === -1 ? GROUP_ORDER.length : i;
}

const FILTER_LABELS: Record<Filter, string> = {
  all: t("All", "command filter"),
  runnable: t("Run", "command filter: runs in the browser"),
  demo: t("Demo", "command filter: runs against a stub"),
  network: t("Needs network", "command filter"),
};

const SEARCH_PLACEHOLDER = t(
  "Search by name, alias, flag, or description…",
  "search box placeholder",
);

function groupKey(groupID: string | undefined): string {
  return groupID || OTHER_GROUP;
}

function groupLabel(key: string): string {
  return GROUP_LABELS[key] ?? key;
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

  // A subcommand files under the group of the top-level command it belongs
  // to, as the root help lists it: `terms import` sits with `terms` under
  // Assets, and `context review` with `context` under Work.
  const rootGroups = useMemo(() => {
    const map = new Map<string, string | undefined>();
    for (const c of commands) if (c.path.length === 1) map.set(c.path[0], c.groupID);
    return map;
  }, [commands]);

  // Group by the top-level command's group id, then order the groups by
  // GROUP_ORDER.
  const grouped = useMemo(() => {
    const map = new Map<string, CommandEntry[]>();
    for (const c of filtered) {
      const key = groupKey(c.path.length > 1 ? rootGroups.get(c.path[0]) : c.groupID);
      const list = map.get(key) ?? [];
      list.push(c);
      map.set(key, list);
    }
    return [...map.entries()].sort(
      ([a], [b]) => groupRank(a) - groupRank(b) || groupLabel(a).localeCompare(groupLabel(b)),
    );
  }, [filtered, rootGroups]);

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

  return (
    <>
      <div className={styles.toolbar}>
        <input
          type="text"
          className={styles.search}
          placeholder={SEARCH_PLACEHOLDER}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className={styles.filterGroup} role="group" aria-label="Filter by runnability">
          {filterButton("all")}
          {filterButton("runnable")}
          {filterButton("demo")}
          {filterButton("network")}
        </div>
      </div>

      <p className={styles.resultCount}>
        {filtered.length} of {commands.length} commands
      </p>

      {grouped.map(([group, items]) => (
        <section key={group} className={styles.groupSection}>
          <h2 className={styles.groupHeading}>{groupLabel(group)}</h2>
          <div className={styles.grid}>
            {items.map((cmd) => (
              <CommandCard key={cmd.id} cmd={cmd} href={commandHref(cmd)} />
            ))}
          </div>
        </section>
      ))}

      {filtered.length === 0 && <p className={styles.empty}>No commands match your search.</p>}
    </>
  );
}

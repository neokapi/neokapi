// The project feed: the query that pages it, and the words a row is made of.
// The server sends kinds, counts and names; every label and unit here comes
// from the catalog, with the language's plural rules.
import { mainCount, type FeedCount, type FeedEntry, type FeedPage } from "@neokapi/bowrain-client";
import { useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import type { TFunction } from "i18next";

import { useAuth } from "./auth";
import { localeName } from "./localeName";

/** Entries per page. */
const PAGE_SIZE = 50;

/** The query key of a project's feed. */
export function feedKey(ws: string, project: string) {
  return ["feed", ws, project] as const;
}

/** A project's feed, newest first, a page at a time. */
export function useFeed(ws: string, project: string) {
  const { client } = useAuth();
  return useInfiniteQuery({
    queryKey: feedKey(ws, project),
    queryFn: ({ pageParam }) =>
      client.feed(ws, project, { limit: PAGE_SIZE, cursor: pageParam || undefined }),
    initialPageParam: "",
    getNextPageParam: (last: FeedPage) => last.next_cursor || undefined,
  });
}

/** Every entry the feed's pages hold. */
export function entriesOf(data: InfiniteData<FeedPage> | undefined): FeedEntry[] {
  const seen = new Set<string>();
  const out: FeedEntry[] = [];
  for (const page of data?.pages ?? []) {
    for (const e of page.entries) {
      if (seen.has(e.id)) continue;
      seen.add(e.id);
      out.push(e);
    }
  }
  return out;
}

const KINDS = [
  "push",
  "convergence_run",
  "review_summary",
  "recipe_change",
  "context_change",
  "change_set",
  "coverage_milestone",
];

const STATES = [
  "converged",
  "parked",
  "failed",
  "canceled",
  "submitted",
  "approved",
  "rejected",
  "merged",
  "created",
  "abandoned",
  "superseded",
];

const UNITS = new Set([
  "files",
  "blocks",
  "passes",
  "locales",
  "produced",
  "decisions",
  "approved",
  "rejected",
  "unreviewed",
  "fixed",
  "bulk_approved",
  "collections_added",
  "collections_removed",
  "coordinates_changed",
  "targets_added",
  "targets_removed",
  "settings_changed",
  "term",
  "term_rule",
  "voice_rule",
  "voice_profile",
  "note",
  "decision",
  "memory",
  "binding",
  "items",
  "add",
  "edit",
  "retire",
  "keep",
  "drop",
]);

/** The kind's label: Push, Run, Review. */
export function kindLabel(t: TFunction, kind: string): string {
  return t(`feed.kind.${KINDS.includes(kind) ? kind : "other"}`);
}

/** A run's outcome or a change set's step, worded; an unknown one as sent. */
export function stateLabel(t: TFunction, state: string): string {
  return STATES.includes(state) ? t(`feed.state.${state}`) : state;
}

/** One count with its unit: "40 blocks", "6 term rules", "90%". */
export function countLabel(t: TFunction, c: FeedCount): string {
  if (c.key === "percent") return t("feed.percent", { count: c.n });
  return t(`feed.unit.${UNITS.has(c.key) ? c.key : "other"}`, { count: c.n });
}

/** A language as the reader names it, else its code. */
export function languageLabel(code: string, uiLanguage: string): string {
  return localeName(code, uiLanguage) ?? code;
}

/** What the row is about, after the kind: the stream, the outcome, the language. */
function objectOf(e: FeedEntry, t: TFunction, uiLanguage: string): string | undefined {
  switch (e.kind) {
    case "push":
      return e.stream;
    case "convergence_run":
    case "change_set":
      return e.state ? stateLabel(t, e.state) : undefined;
    case "review_summary":
      return e.locales?.[0] ? languageLabel(e.locales[0], uiLanguage) : undefined;
    case "recipe_change": {
      const cols = e.diff?.collections ?? [];
      return cols.length === 1 ? cols[0].name : undefined;
    }
    default:
      return undefined;
  }
}

/** The row's primary line: "Push · main", "Run · converged", "nb · 90%". */
export function entryTitle(e: FeedEntry, t: TFunction, uiLanguage: string): string {
  if (e.kind === "coverage_milestone" && e.locales?.[0]) {
    const pct = e.counts.percent;
    const lang = languageLabel(e.locales[0], uiLanguage);
    return pct !== undefined ? `${lang} · ${t("feed.percent", { count: pct })}` : lang;
  }
  const object = objectOf(e, t, uiLanguage);
  return object ? `${kindLabel(t, e.kind)} · ${object}` : kindLabel(t, e.kind);
}

/** Who did it, as the row names them. */
export function actorLabel(e: FeedEntry, t: TFunction): string {
  return e.actor.name || e.on_behalf_of?.name || t("feed.someone");
}

/** The row's meta line: the actor, the main count and the time, at most three facts. */
export function entryMeta(e: FeedEntry, t: TFunction, uiLanguage: string): string {
  const facts = [actorLabel(e, t)];
  const main = e.kind === "coverage_milestone" ? undefined : mainCount(e);
  if (main) facts.push(countLabel(t, main));
  facts.push(timeOf(e.at, uiLanguage));
  return facts.join(" · ");
}

/** The time of day of an entry, in the reader's language. */
export function timeOf(iso: string, uiLanguage: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  try {
    return new Intl.DateTimeFormat(uiLanguage, { hour: "numeric", minute: "2-digit" }).format(d);
  } catch {
    return d.toISOString().slice(11, 16);
  }
}

/** A day heading: Today, Yesterday, else the date. */
export function dayLabel(daysAgo: number, date: Date, t: TFunction, uiLanguage: string): string {
  if (daysAgo <= 0) return t("feed.today");
  if (daysAgo === 1) return t("feed.yesterday");
  try {
    return new Intl.DateTimeFormat(uiLanguage, {
      weekday: "long",
      day: "numeric",
      month: "long",
    }).format(date);
  } catch {
    return date.toDateString();
  }
}

/** The deck an entry opens, when the app has the screen it links to. */
export function deckLocaleOf(e: FeedEntry): string | undefined {
  if (e.link.screen === "review_deck" || e.link.screen === "language") {
    return e.link.locale ?? e.locales?.[0];
  }
  return undefined;
}

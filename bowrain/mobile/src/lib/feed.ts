// The project feed: the query that pages it, and the words a row is made of.
// The server sends kinds, counts and names; every label and unit here comes
// from the catalog, with the language's plural rules.
import {
  ageOf,
  mainCount,
  targetChanges,
  type FeedCard,
  type FeedCount,
  type FeedEntry,
  type FeedPage,
} from "@neokapi/bowrain-client";
import { useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import type { TFunction } from "i18next";

import { useAuth } from "./auth";
import i18n from "./i18n";
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
  // A runtime without Intl.DisplayNames (Hermes on some versions) names the
  // common languages from the catalog, which each UI language translates.
  return (
    localeName(code, uiLanguage) ??
    i18n.t(`languageNames.${code}`, {
      defaultValue: i18n.t(`languageNames.${code.split(/[-_]/)[0]}`, { defaultValue: code }),
    })
  );
}

/** Items joined the way the UI language lists them: "A, B and C". */
export function listOf(items: string[], t: TFunction, uiLanguage: string): string {
  if (items.length <= 1) return items[0] ?? "";
  try {
    if (typeof Intl.ListFormat === "function") {
      return new Intl.ListFormat(uiLanguage, { type: "conjunction" }).format(items);
    }
  } catch {
    // fall through to the catalog's own joining
  }
  if (items.length === 2) return t("feed.list.two", { a: items[0], b: items[1] });
  return t("feed.list.many", {
    rest: items.slice(0, -1).join(", "),
    last: items[items.length - 1],
  });
}

/** The compact time of an entry: now, 5m, 2h, yesterday, or the date. */
export function ageLabel(iso: string, t: TFunction, uiLanguage: string, now = new Date()): string {
  const age = ageOf(iso, now);
  switch (age.unit) {
    case "now":
      return t("feed.time.now");
    case "minutes":
      return t("feed.time.minutes", { count: age.n });
    case "hours":
      return t("feed.time.hours", { count: age.n });
    case "yesterday":
      return t("feed.time.yesterday");
    case "date":
      try {
        return new Intl.DateTimeFormat(uiLanguage, { day: "numeric", month: "short" }).format(
          age.date,
        );
      } catch {
        return age.date.toDateString();
      }
  }
}

const CONTEXT_OP_KEYS: Record<string, string> = {
  add: "contextAdd",
  edit: "contextEdit",
  keep: "contextKeep",
  drop: "contextDrop",
  retire: "contextRetire",
};

const CONTEXT_OPS = ["add", "edit", "keep", "drop", "retire"];

const CHANGE_SET_KEYS: Record<string, string> = {
  created: "changeSetCreated",
  submitted: "changeSetSubmitted",
  approved: "changeSetApproved",
  rejected: "changeSetRejected",
  merged: "changeSetMerged",
  abandoned: "changeSetAbandoned",
  superseded: "changeSetSuperseded",
};

/**
 * What the actor did, as the sentence after their name: "added Japanese and
 * Korean", "proposed 6 term rules", "drafted 40 translations in 3 languages".
 */
export function actionOf(e: FeedEntry, t: TFunction, uiLanguage: string): string {
  const c = e.counts ?? {};
  switch (e.kind) {
    case "push": {
      if (c.blocks === undefined) return t("feed.action.pushSync");
      return e.stream
        ? t("feed.action.push", { count: c.blocks, stream: e.stream })
        : t("feed.action.pushNoStream", { count: c.blocks });
    }
    case "convergence_run": {
      if (e.state === "parked") return t("feed.action.runParked");
      if (e.state === "failed") return t("feed.action.runFailed");
      if (e.state === "canceled") return t("feed.action.runCanceled");
      const produced = c.produced ?? 0;
      if (produced === 0) return t("feed.action.runNothing");
      const langs = e.locales?.length ?? c.locales ?? 0;
      const languages =
        langs === 1 && e.locales
          ? languageLabel(e.locales[0], uiLanguage)
          : t("feed.unit.locales", { count: langs });
      return t("feed.action.runDrafted", { count: produced, languages });
    }
    case "review_summary": {
      const parts: string[] = [];
      // Approvals one by one and in bulk read as one count; the preview's
      // split bar keeps the detail.
      const approved = (c.approved ?? 0) + (c.bulk_approved ?? 0);
      if (approved) parts.push(t("feed.action.approved", { count: approved }));
      if (c.fixed) parts.push(t("feed.action.fixed", { count: c.fixed }));
      if (c.rejected) parts.push(t("feed.action.rejected", { count: c.rejected }));
      if (c.unreviewed) parts.push(t("feed.action.sentBack", { count: c.unreviewed }));
      const language = e.locales?.[0] ? languageLabel(e.locales[0], uiLanguage) : "";
      return t("feed.action.review", { parts: listOf(parts, t, uiLanguage), language });
    }
    case "recipe_change": {
      const parts: string[] = [];
      const { added, removed } = targetChanges(e);
      if (added.length) {
        parts.push(
          t("feed.action.addedLanguages", {
            languages: listOf(
              added.map((l) => languageLabel(l, uiLanguage)),
              t,
              uiLanguage,
            ),
          }),
        );
      }
      if (removed.length) {
        parts.push(
          t("feed.action.removedLanguages", {
            languages: listOf(
              removed.map((l) => languageLabel(l, uiLanguage)),
              t,
              uiLanguage,
            ),
          }),
        );
      }
      for (const col of e.diff?.collections ?? []) {
        const key =
          col.change === "added"
            ? "addedCollection"
            : col.change === "removed"
              ? "removedCollection"
              : "movedCollection";
        parts.push(t(`feed.action.${key}`, { name: col.name }));
      }
      if (e.diff?.settings?.length) {
        parts.push(t("feed.action.changedSettings", { count: e.diff.settings.length }));
      }
      return parts.length ? listOf(parts, t, uiLanguage) : t("feed.action.other");
    }
    case "context_change": {
      const op = CONTEXT_OPS.find((o) => (c[o] ?? 0) > 0) ?? "add";
      const main = mainCount(e);
      const items = main ? countLabel(t, main) : "";
      const key = op === "add" && e.actor.kind === "agent" ? "contextPropose" : CONTEXT_OP_KEYS[op];
      return t(`feed.action.${key}`, { items });
    }
    case "change_set": {
      const key = CHANGE_SET_KEYS[e.state ?? ""] ?? "changeSetSubmitted";
      return t(`feed.action.${key}`, { name: e.name || t("feed.action.aChangeSet") });
    }
    case "coverage_milestone": {
      const language = e.locales?.[0] ? languageLabel(e.locales[0], uiLanguage) : "";
      return t("feed.action.milestone", { language, percent: c.percent ?? 0 });
    }
    default:
      return t("feed.action.other");
  }
}

const FOLDED_KEYS: Record<string, string> = {
  push: "foldedPush",
  convergence_run: "foldedRun",
  context_change: "foldedContext",
  change_set: "foldedChangeSet",
};

/** The sentence of a folded card: "ran 3 times". */
export function foldedActionOf(card: FeedCard, t: TFunction, uiLanguage: string): string {
  const lead = card.entries[0];
  const key = FOLDED_KEYS[lead.kind];
  if (card.entries.length < 2 || !key) return actionOf(lead, t, uiLanguage);
  return t(`feed.action.${key}`, { count: card.entries.length });
}

/** The name a card shows for its actor. */
export function actorName(e: FeedEntry, t: TFunction): string {
  if (e.actor.kind === "tool" && e.actor.name) {
    return e.actor.name.charAt(0).toUpperCase() + e.actor.name.slice(1);
  }
  return e.actor.name || e.on_behalf_of?.name || t("feed.someone");
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

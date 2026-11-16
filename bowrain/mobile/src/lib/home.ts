// The Home tab's view of the review inbox (GET /auth/me/review): what waits
// for the person, totalled per workspace and per project, and the one deck
// the Home screen offers first. The server counts and orders the inbox; this
// file only sums and looks up what it returned.
import type { ContextInboxEntry, ReviewHome, ReviewInboxLanguage } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";

import { useAuth } from "./auth";
import type { RecentProject } from "./recentProject";

/** The query key the Review tab and the Home tab share. */
export const inboxKey = ["review-inbox"] as const;

/** The review inbox, in the cache the Review tab reads too. */
export function useReviewHome() {
  const { client } = useAuth();
  return useQuery({ queryKey: inboxKey, queryFn: () => client.reviewHome() });
}

/** What waits in one project, one workspace, or everywhere. */
export interface Waiting {
  /** Translations waiting, over every language. */
  translations: number;
  /** Context items waiting. */
  context: number;
  /** Languages with a translation waiting. */
  languages: number;
}

/** What waits in one project, with the inbox's entries for it. */
export interface ProjectWaiting extends Waiting {
  byLocale: Map<string, ReviewInboxLanguage>;
  contextEntry?: ContextInboxEntry;
}

export interface HomeSummary {
  total: Waiting;
  byWorkspace: Map<string, Waiting>;
  /** Keyed by `projectKey(workspace, project)`. */
  byProject: Map<string, ProjectWaiting>;
}

const none = (): Waiting => ({ translations: 0, context: 0, languages: 0 });

export const projectKey = (ws: string, project: string) => `${ws}/${project}`;

/** Total the inbox per workspace and per project. */
export function summarize(home: ReviewHome | null | undefined): HomeSummary {
  const total = none();
  const byWorkspace = new Map<string, Waiting>();
  const byProject = new Map<string, ProjectWaiting>();
  const ws = (slug: string) => {
    let w = byWorkspace.get(slug);
    if (!w) {
      w = none();
      byWorkspace.set(slug, w);
    }
    return w;
  };
  const proj = (slug: string, id: string) => {
    const k = projectKey(slug, id);
    let p = byProject.get(k);
    if (!p) {
      p = { ...none(), byLocale: new Map() };
      byProject.set(k, p);
    }
    return p;
  };
  for (const l of home?.languages ?? []) {
    if (l.waiting <= 0) continue;
    const p = proj(l.workspace, l.project);
    for (const w of [total, ws(l.workspace), p]) {
      w.translations += l.waiting;
      w.languages += 1;
    }
    p.byLocale.set(l.locale, l);
  }
  for (const c of home?.context ?? []) {
    if (c.waiting <= 0) continue;
    const p = proj(c.workspace, c.project);
    for (const w of [total, ws(c.workspace), p]) w.context += c.waiting;
    p.contextEntry = c;
  }
  return { total, byWorkspace, byProject };
}

/** The deck the Home screen offers first. */
export type NextUp =
  | { kind: "language"; entry: ReviewInboxLanguage; more: number }
  | { kind: "context"; entry: ContextInboxEntry; more: number };

/**
 * The first deck to offer. Source context comes before translations, since
 * the translations are drafted from it: a project whose context waits,
 * preferring the project the person last opened. With no context waiting, a
 * language in the project last opened, and otherwise the inbox's first
 * language, which the server puts first because the most waits there.
 * `more` counts the other decks waiting.
 */
export function nextUp(
  home: ReviewHome | null | undefined,
  recent?: RecentProject | null,
): NextUp | null {
  const languages = (home?.languages ?? []).filter((l) => l.waiting > 0);
  const context = (home?.context ?? []).filter((c) => c.waiting > 0);
  const more = languages.length + context.length - 1;
  const inRecent = <T extends { workspace: string; project: string }>(list: T[]) =>
    recent
      ? list.find((e) => e.workspace === recent.ws && e.project === recent.project)
      : undefined;
  const ctx = inRecent(context) ?? context[0];
  if (ctx) return { kind: "context", entry: ctx, more };
  const language = inRecent(languages) ?? languages[0];
  if (language) return { kind: "language", entry: language, more };
  return null;
}

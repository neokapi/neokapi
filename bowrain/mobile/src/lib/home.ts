// The Home tab's view of the review inbox (GET /auth/me/review): what waits
// for the person, totalled per workspace and per project, and the project
// whose plan Home shows first. The server counts and orders the inbox and
// names each plan's next step; this file only sums and looks up what it
// returned.
import {
  nextPlanStep,
  type ContextInboxEntry,
  type PlanStep,
  type PlanWait,
  type ReviewHome,
  type ReviewInboxLanguage,
  type ReviewInboxProject,
} from "@neokapi/bowrain-client";
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

/**
 * The first card on Home: the next step of one project's plan, as the server
 * names it. `waiting` lists the steps the server skipped because they only
 * wait; `more` counts the other decks waiting, which the Review tab lists.
 *
 * - setup: the person has not chosen their languages in the project.
 * - step: the next step opens its own screen (coordinates, context, policy),
 *   or its deck holds nothing in a followed language.
 * - deck: the next step is reviewed in a language deck (exceptions, pages).
 * - waiting: no step can be acted on, and at least one waits.
 */
export type HomeStep =
  | { kind: "setup"; project: ReviewInboxProject; waiting: PlanWait[]; more: number }
  | {
      kind: "step";
      project: ReviewInboxProject;
      step: PlanStep;
      waiting: PlanWait[];
      more: number;
    }
  | {
      kind: "deck";
      project: ReviewInboxProject;
      step: PlanStep;
      entry: ReviewInboxLanguage;
      waiting: PlanWait[];
      more: number;
    }
  | { kind: "waiting"; project: ReviewInboxProject; waiting: PlanWait[]; more: number };

/** A project the person can act on: one to set up, or one whose plan names a next step. */
function actionable(p: ReviewInboxProject): boolean {
  return !p.saved || (p.plan?.next ?? null) !== null;
}

/** A project with a step that only waits. */
function waits(p: ReviewInboxProject): boolean {
  return (p.plan?.waiting?.length ?? 0) > 0;
}

/**
 * The card Home shows first, or null when nothing waits. The project is the
 * one the person last opened when it has a next step, else the first in the
 * inbox's order with one; with none to act on, a project whose steps wait.
 * The step is the plan's `next`, which the server computes.
 */
export function homeStep(
  home: ReviewHome | null | undefined,
  recent?: RecentProject | null,
): HomeStep | null {
  const projects = home?.projects ?? [];
  const languages = (home?.languages ?? []).filter((l) => l.waiting > 0);
  const decks = languages.length + (home?.context ?? []).filter((c) => c.waiting > 0).length;
  const isRecent = (p: ReviewInboxProject) =>
    !!recent && p.workspace === recent.ws && p.project === recent.project;
  const project =
    projects.find((p) => isRecent(p) && actionable(p)) ??
    projects.find(actionable) ??
    projects.find((p) => isRecent(p) && waits(p)) ??
    projects.find(waits);
  if (!project) return null;
  const waiting = project.plan?.waiting ?? [];
  if (!project.saved) return { kind: "setup", project, waiting, more: decks };
  const step = project.plan ? nextPlanStep(project.plan.steps, project.plan.next) : undefined;
  if (!step) return { kind: "waiting", project, waiting, more: decks };
  if (step.level === "exceptions" || step.level === "interfaces") {
    const entry = languages.find(
      (l) => l.workspace === project.workspace && l.project === project.project,
    );
    if (entry) return { kind: "deck", project, step, entry, waiting, more: decks - 1 };
  }
  return { kind: "step", project, step, waiting, more: decks };
}

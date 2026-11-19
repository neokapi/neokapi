// The languages a person follows in a project, and the project's plan in
// brief as the review inbox carries it (GET /:ws/:id/review/languages and
// GET /auth/me/review). A followed list filters the project's shared queues
// for one person; it never copies work. The server decides what is allowed
// and counts every step; this file maps its answers.

import type { PlanLevel, PlanSettle } from "./plan.ts";

/** The languages a person follows in one project, and the project's own. */
export interface ReviewLanguages {
  /** The languages the person follows: the saved list, or every allowed one when none is saved. */
  followed: string[];
  /** True once the person has saved a list for the project. */
  saved: boolean;
  /** The project's target languages the person may review. */
  allowed: string[];
  /** The project's source language. */
  source: string;
  /** The project's target languages as the last push declared them. */
  project: string[];
  /** The target languages a recipe change waiting for the next sync sets, if one waits. */
  pendingProject?: string[];
  /** True when the person may turn the project's languages on and off. */
  canManage: boolean;
}

export interface ServerReviewLanguages {
  followed?: string[] | null;
  saved?: boolean;
  allowed?: string[] | null;
  source?: string;
  project?: string[] | null;
  pending_project?: string[] | null;
  can_manage?: boolean;
}

export function reviewLanguagesFromServer(l: ServerReviewLanguages): ReviewLanguages {
  return {
    followed: l.followed ?? [],
    saved: l.saved ?? false,
    allowed: l.allowed ?? [],
    source: l.source ?? "",
    project: l.project ?? [],
    pendingProject: l.pending_project ?? undefined,
    canManage: l.can_manage ?? false,
  };
}

/** The target languages a project will have once its pending recipe change applies. */
export function projectLanguagesAfterSync(l: ReviewLanguages): string[] {
  return l.pendingProject ?? l.project;
}

/** One level of a project's plan in brief. */
export interface PlanStep {
  level: PlanLevel;
  /** What waits at the level. */
  count: number;
  /** The team's progress through the level, 0 to 100. */
  progress: number;
  done: boolean;
  /** What changed at the level after the team finished it. */
  changed: number;
  /** The policy level's settle state. */
  settle?: PlanSettle;
}

/** A project's review plan in brief: one step per level, in order. */
export interface PlanSummary {
  involvement: string;
  /** The first level with something waiting, or null when nothing does. */
  next: PlanLevel | null;
  steps: PlanStep[];
}

/** One project of the review inbox, with the languages the person follows there. */
export interface ReviewInboxProject {
  workspace: string;
  workspaceName: string;
  project: string;
  projectName: string;
  followed: string[];
  saved: boolean;
  canManage: boolean;
  /** Context cards and translations in followed languages waiting for the person. */
  waiting: number;
  /** The plan in brief; the server computes it only for a project with work waiting. */
  plan?: PlanSummary;
}

interface ServerPlanStep {
  level: string;
  count?: number;
  progress?: number;
  done?: boolean;
  changed?: number;
  settle?: string;
}

export interface ServerReviewInboxProject {
  workspace: string;
  workspace_name: string;
  project: string;
  project_name: string;
  followed?: string[] | null;
  saved?: boolean;
  can_manage?: boolean;
  waiting?: number;
  plan?: { involvement?: string; next?: string; steps?: ServerPlanStep[] | null } | null;
}

const LEVELS: readonly PlanLevel[] = [
  "coordinates",
  "key_language",
  "exceptions",
  "interfaces",
  "policy",
];
const SETTLE: readonly PlanSettle[] = ["first_pass", "policy", "held", "none"];

function levelOf(v: string | undefined): PlanLevel | null {
  return (LEVELS as readonly string[]).includes(v ?? "") ? (v as PlanLevel) : null;
}

export function inboxProjectFromServer(p: ServerReviewInboxProject): ReviewInboxProject {
  const steps: PlanStep[] = [];
  for (const s of p.plan?.steps ?? []) {
    const level = levelOf(s.level);
    if (!level) continue;
    steps.push({
      level,
      count: s.count ?? 0,
      progress: Math.max(0, Math.min(100, s.progress ?? 0)),
      done: s.done ?? false,
      changed: s.changed ?? 0,
      settle: (SETTLE as readonly string[]).includes(s.settle ?? "")
        ? (s.settle as PlanSettle)
        : undefined,
    });
  }
  return {
    workspace: p.workspace,
    workspaceName: p.workspace_name,
    project: p.project,
    projectName: p.project_name,
    followed: p.followed ?? [],
    saved: p.saved ?? false,
    canManage: p.can_manage ?? false,
    waiting: p.waiting ?? 0,
    plan: p.plan
      ? { involvement: p.plan.involvement ?? "context", next: levelOf(p.plan.next), steps }
      : undefined,
  };
}

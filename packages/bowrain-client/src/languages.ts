// The languages a person follows in a project, and the project's plan in
// brief as the review inbox carries it (GET /:ws/:id/review/languages and
// GET /auth/me/review). A followed list filters the project's shared queues
// for one person; it never copies work. The server decides what is allowed
// and counts every step; this file maps its answers.

import {
  planLevelOf,
  planStepsFromServer,
  planWaitingFromServer,
  type PlanHeldReason,
  type PlanLevel,
  type PlanStep,
  type PlanWait,
  type ServerPlanStep,
  type ServerPlanWait,
} from "./plan.ts";

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

/** The translations one language's deck holds back, and why. */
export interface PlanHeld {
  locale: string;
  count: number;
  reason: PlanHeldReason;
}

/** A project's review plan in brief: one step per level, in order. */
export interface PlanSummary {
  involvement: string;
  /** The first level the person can act on, or null when none can be. */
  next: PlanLevel | null;
  /**
   * The levels skipped for `next` because they only wait, with the reason,
   * so a client can show them beside the next step.
   */
  waiting: PlanWait[];
  steps: PlanStep[];
  /** Per followed language, the translations held until the context is decided. */
  held: PlanHeld[];
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
  /**
   * The plan in brief; the server computes it only for a project with work
   * waiting or translations held.
   */
  plan?: PlanSummary;
}

export interface ServerPlanHeld {
  locale: string;
  count: number;
  reason?: string;
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
  plan?: {
    involvement?: string;
    next?: string;
    waiting?: ServerPlanWait[] | null;
    steps?: ServerPlanStep[] | null;
    held?: ServerPlanHeld[] | null;
  } | null;
}

/** The held translations per language; a reason this client does not know is left out. */
export function planHeldFromServer(held: ServerPlanHeld[] | null | undefined): PlanHeld[] {
  return (held ?? [])
    .filter((h) => (h.reason ?? "context") === "context" && h.count > 0)
    .map((h) => ({ locale: h.locale, count: h.count, reason: "context" }));
}

export function inboxProjectFromServer(p: ServerReviewInboxProject): ReviewInboxProject {
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
      ? {
          involvement: p.plan.involvement ?? "context",
          next: planLevelOf(p.plan.next),
          waiting: planWaitingFromServer(p.plan.waiting),
          steps: planStepsFromServer(p.plan.steps),
          held: planHeldFromServer(p.plan.held),
        }
      : undefined,
  };
}

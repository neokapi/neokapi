// The languages a person follows in a project, and the project's plan in
// brief as the review inbox carries it (GET /:ws/:id/review/languages and
// GET /auth/me/review). A followed list filters the project's shared queues
// for one person; it never copies work. The server decides what is allowed
// and counts every step; this file maps its answers.

import type { PlanLevel, PlanScan, PlanSettle } from "./plan.ts";

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

/**
 * A plan level's state. not_started: nothing to work from yet, such as a key
 * language with no scan and no concepts. open: something waits. done:
 * nothing waits. attention: done, then changed, such as coordinates changed
 * since the last confirmation.
 */
export type PlanStepState = "not_started" | "open" | "done" | "attention";

/** One level of a project's plan in brief. */
export interface PlanStep {
  level: PlanLevel;
  state: PlanStepState;
  /** What waits at the level. */
  count: number;
  /** The team's progress through the level, 0 to 100. */
  progress: number;
  done: boolean;
  /** What changed at the level after the team finished it. */
  changed: number;
  /** The policy level's settle state. */
  settle?: PlanSettle;
  /** The key language's last term scan state, if a scan was started. */
  scan?: PlanScan["status"];
}

/** Why a language deck holds translations back: the key language is open. */
export type PlanHeldReason = "key_language";

/** The translations one language's deck holds back, and why. */
export interface PlanHeld {
  locale: string;
  count: number;
  reason: PlanHeldReason;
}

/** A project's review plan in brief: one step per level, in order. */
export interface PlanSummary {
  involvement: string;
  /** The first level with something waiting, or null when nothing does. */
  next: PlanLevel | null;
  steps: PlanStep[];
  /** Per followed language, the translations held until the key language is decided. */
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

export interface ServerPlanStep {
  level: string;
  state?: string;
  count?: number;
  progress?: number;
  done?: boolean;
  changed?: number;
  settle?: string;
  scan?: string;
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
    steps?: ServerPlanStep[] | null;
    held?: ServerPlanHeld[] | null;
  } | null;
}

const LEVELS: readonly PlanLevel[] = [
  "coordinates",
  "key_language",
  "exceptions",
  "interfaces",
  "policy",
];
const SETTLE: readonly PlanSettle[] = ["first_pass", "policy", "held", "none"];
const STATES: readonly PlanStepState[] = ["not_started", "open", "done", "attention"];
const SCAN_STATUS: readonly PlanScan["status"][] = ["queued", "processing", "completed", "failed"];

function levelOf(v: string | undefined): PlanLevel | null {
  return (LEVELS as readonly string[]).includes(v ?? "") ? (v as PlanLevel) : null;
}

/**
 * A step's state as the server reports it. An older server sends none, so
 * the state follows `done` and `changed`, as that server meant them.
 */
function stateOf(s: ServerPlanStep): PlanStepState {
  if ((STATES as readonly string[]).includes(s.state ?? "")) return s.state as PlanStepState;
  if ((s.changed ?? 0) > 0) return "attention";
  return s.done ? "done" : "open";
}

/** The plan's steps, in order, leaving out a level this client does not know. */
export function planStepsFromServer(steps: ServerPlanStep[] | null | undefined): PlanStep[] {
  const out: PlanStep[] = [];
  for (const s of steps ?? []) {
    const level = levelOf(s.level);
    if (!level) continue;
    out.push({
      level,
      state: stateOf(s),
      count: s.count ?? 0,
      progress: Math.max(0, Math.min(100, s.progress ?? 0)),
      done: s.done ?? false,
      changed: s.changed ?? 0,
      settle: (SETTLE as readonly string[]).includes(s.settle ?? "")
        ? (s.settle as PlanSettle)
        : undefined,
      scan: (SCAN_STATUS as readonly string[]).includes(s.scan ?? "")
        ? (s.scan as PlanScan["status"])
        : undefined,
    });
  }
  return out;
}

/** The held translations per language; a reason this client does not know is left out. */
export function planHeldFromServer(held: ServerPlanHeld[] | null | undefined): PlanHeld[] {
  return (held ?? [])
    .filter((h) => (h.reason ?? "key_language") === "key_language" && h.count > 0)
    .map((h) => ({ locale: h.locale, count: h.count, reason: "key_language" }));
}

/** The step the plan names next, or undefined when nothing waits. */
export function nextPlanStep(steps: PlanStep[], next: PlanLevel | null): PlanStep | undefined {
  return next ? steps.find((s) => s.level === next) : undefined;
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
          next: levelOf(p.plan.next),
          steps: planStepsFromServer(p.plan.steps),
          held: planHeldFromServer(p.plan.held),
        }
      : undefined,
  };
}

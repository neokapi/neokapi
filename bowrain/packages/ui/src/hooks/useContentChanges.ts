import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ChangeFinding, ChangeGate, ChangeOp, CurrentEdition } from "@neokapi/contract-types";
import {
  ChangeRefusedError,
  contentChangeSet,
  readOutcome,
  revisionAfter,
  type ChangeResult,
} from "../api/contentChanges";
import { useEditorApi } from "./useEditorApi";

/** What a person was doing when the translation moved under them. */
export type StaleAction = "save" | "establish" | "reject" | "withdraw";

/** What the stale prompt shows. */
export interface StalePrompt {
  action: StaleAction;
  /** The language of the translation. */
  locale: string;
  /** For a save: the wording the person meant to apply, in placeholder form. */
  mine?: string;
  /** The translation as it now stands. */
  current: CurrentEdition;
}

/** The person's answer: apply the change to the translation that stands, or keep it. */
export type StaleChoice = "reapply" | "keep";

/** What the findings prompt shows when a check refuses a save. */
export interface FindingsPrompt {
  /** The language of the translation. */
  locale: string;
  /** The wording the person saved, in placeholder form. */
  mine?: string;
  /** The failing findings the save would introduce. */
  findings: ChangeFinding[];
}

/** The person's answer: save over the findings, or go back to the wording. */
export type FindingsChoice = "override" | "revise";

/** What became of a change the person made. */
export type CommitOutcome =
  /** The change landed; `after` is the revision the translation now holds. */
  | { status: "applied"; result: ChangeResult; after?: string }
  /** The translation moved and the person kept it as it stands. */
  | { status: "kept"; current: CurrentEdition }
  /** A check refused the save and the person went back to their wording. */
  | { status: "revise"; findings: ChangeFinding[] };

/** The open stale prompt, as the dialog takes it. */
export interface StaleDialogState extends StalePrompt {
  onReapply: () => void;
  onKeep: () => void;
}

/** The open findings prompt, as the dialog takes it. */
export interface FindingsDialogState extends FindingsPrompt {
  onOverride: () => void;
  onRevise: () => void;
}

/** One open prompt and the answer the waiting commit takes from it. */
type Pending =
  | { kind: "stale"; prompt: StalePrompt; resolve: (choice: StaleChoice) => void }
  | { kind: "findings"; prompt: FindingsPrompt; resolve: (choice: FindingsChoice) => void };

/** Settle a prompt with the answer that changes nothing. */
function dismiss(p: Pending) {
  if (p.kind === "stale") p.resolve("keep");
  else p.resolve("revise");
}

/**
 * Sends a person's changes to a project's active stream as change sets, and
 * settles the refusals a person answers. A stale refusal means the translation
 * changed since the surface rendered it: they see it as it stands and either
 * apply their change to it or keep it. A save a check refuses shows the
 * findings: they go back to their wording, or save it over the findings, which
 * the change records as their override. `staleDialog` and `findingsDialog` are
 * the open prompts, for `StaleChangeDialog` and `CheckFindingsDialog`.
 */
export function useContentChanges(projectId: string) {
  const { applyChanges } = useEditorApi();
  const [pending, setPending] = useState<Pending | null>(null);
  // The open prompt, read outside render: a new prompt and an unmount settle
  // it, so no commit waits on a prompt nobody can answer.
  const open = useRef<Pending | null>(null);

  const show = useCallback((next: Pending) => {
    const prev = open.current;
    open.current = next;
    setPending(next);
    if (prev) dismiss(prev);
  }, []);

  const settle = useCallback((p: Pending, answer: () => void) => {
    // A second click on a prompt already answered does nothing.
    if (open.current !== p) return;
    open.current = null;
    setPending(null);
    answer();
  }, []);

  useEffect(
    () => () => {
      const prev = open.current;
      open.current = null;
      if (prev) dismiss(prev);
    },
    [],
  );

  const askStale = useCallback(
    (prompt: StalePrompt) =>
      new Promise<StaleChoice>((resolve) => show({ kind: "stale", prompt, resolve })),
    [show],
  );
  const askFindings = useCallback(
    (prompt: FindingsPrompt) =>
      new Promise<FindingsChoice>((resolve) => show({ kind: "findings", prompt, resolve })),
    [show],
  );

  /**
   * Send one operation that names the revision the surface rendered. `build`
   * makes it for a revision: first `ifMatch`, then, when the person chooses to
   * apply it to the translation that stands, that translation's revision. A
   * save a check refuses is offered to the person to save anyway; any other
   * refusal throws `ChangeRefusedError`.
   */
  const commit = useCallback(
    async (
      build: (ifMatch: string) => ChangeOp,
      ifMatch: string,
      prompt: Omit<StalePrompt, "current">,
    ): Promise<CommitOutcome> => {
      let rev = ifMatch;
      let gate: ChangeGate | undefined;
      for (;;) {
        const result = await applyChanges(projectId, contentChangeSet([build(rev)], { gate }));
        const outcome = readOutcome(result);
        switch (outcome.status) {
          case "applied":
            return { status: "applied", result, after: revisionAfter(result) };
          case "stale": {
            const choice = await askStale({ ...prompt, current: outcome.current });
            if (choice === "keep") return { status: "kept", current: outcome.current };
            rev = outcome.current.rev;
            continue;
          }
          case "gate_failed": {
            // Overriding a check is a person's call on their own wording, so
            // only a save offers it; a decision a check refuses is an error.
            if (prompt.action !== "save" || gate === "report") {
              throw new ChangeRefusedError(result, outcome.error);
            }
            const choice = await askFindings({
              locale: prompt.locale,
              mine: prompt.mine,
              findings: outcome.findings,
            });
            if (choice === "revise") return { status: "revise", findings: outcome.findings };
            gate = "report";
            continue;
          }
          case "refused":
            throw new ChangeRefusedError(result, outcome.error);
        }
      }
    },
    [applyChanges, projectId, askStale, askFindings],
  );

  /**
   * Send operations that name no revision (notes, entity marks). Any refusal
   * throws `ChangeRefusedError`.
   */
  const apply = useCallback(
    async (ops: ChangeOp[]): Promise<ChangeResult> => {
      const result = await applyChanges(projectId, contentChangeSet(ops));
      const outcome = readOutcome(result);
      switch (outcome.status) {
        case "applied":
          return result;
        case "stale":
          throw new ChangeRefusedError(result, {
            code: "stale",
            message: outcome.op.error?.message ?? "the content changed",
          });
        default:
          throw new ChangeRefusedError(result, outcome.error);
      }
    },
    [applyChanges, projectId],
  );

  const staleDialog = useMemo<StaleDialogState | null>(() => {
    if (pending?.kind !== "stale") return null;
    const p = pending;
    return {
      ...p.prompt,
      onReapply: () => settle(p, () => p.resolve("reapply")),
      onKeep: () => settle(p, () => p.resolve("keep")),
    };
  }, [pending, settle]);

  const findingsDialog = useMemo<FindingsDialogState | null>(() => {
    if (pending?.kind !== "findings") return null;
    const p = pending;
    return {
      ...p.prompt,
      onOverride: () => settle(p, () => p.resolve("override")),
      onRevise: () => settle(p, () => p.resolve("revise")),
    };
  }, [pending, settle]);

  return { commit, apply, staleDialog, findingsDialog };
}

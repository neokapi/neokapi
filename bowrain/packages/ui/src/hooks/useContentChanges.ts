import { useCallback, useMemo, useRef, useState } from "react";
import type { ChangeOp, CurrentEdition } from "@neokapi/contract-types";
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

/** What became of a change the person made. */
export type CommitOutcome =
  /** The change landed; `after` is the revision the translation now holds. */
  | { status: "applied"; result: ChangeResult; after?: string }
  /** The translation moved and the person kept it as it stands. */
  | { status: "kept"; current: CurrentEdition };

/** The open stale prompt, as the dialog takes it. */
export interface StaleDialogState extends StalePrompt {
  onReapply: () => void;
  onKeep: () => void;
}

/**
 * Sends a person's changes to a project's active stream as change sets, and
 * settles a stale refusal with them: the translation changed since the surface
 * rendered it, so they see it as it stands and either apply their change to it
 * or keep it. `staleDialog` is the open prompt, for `StaleChangeDialog`.
 */
export function useContentChanges(projectId: string) {
  const { applyChanges } = useEditorApi();
  const [pending, setPending] = useState<{
    prompt: StalePrompt;
    resolve: (choice: StaleChoice) => void;
  } | null>(null);
  // The settle callbacks resolve the promise the commit is waiting on; a ref
  // keeps a second click from resolving it twice.
  const settled = useRef(false);

  const ask = useCallback(
    (prompt: StalePrompt) =>
      new Promise<StaleChoice>((resolve) => {
        settled.current = false;
        setPending({ prompt, resolve });
      }),
    [],
  );

  /**
   * Send one operation that names the revision the surface rendered. `build`
   * makes it for a revision: first `ifMatch`, then, when the person chooses to
   * apply it to the translation that stands, that translation's revision. A
   * refusal other than a stale revision throws `ChangeRefusedError`.
   */
  const commit = useCallback(
    async (
      build: (ifMatch: string) => ChangeOp,
      ifMatch: string,
      prompt: Omit<StalePrompt, "current">,
    ): Promise<CommitOutcome> => {
      let rev = ifMatch;
      for (;;) {
        const result = await applyChanges(projectId, contentChangeSet([build(rev)]));
        const outcome = readOutcome(result);
        if (outcome.status === "applied") {
          return { status: "applied", result, after: revisionAfter(result) };
        }
        if (outcome.status === "refused") throw new ChangeRefusedError(result, outcome.error);
        const choice = await ask({ ...prompt, current: outcome.current });
        if (choice === "keep") return { status: "kept", current: outcome.current };
        rev = outcome.current.rev;
      }
    },
    [applyChanges, projectId, ask],
  );

  /**
   * Send operations that name no revision (notes, entity marks). Any refusal
   * throws `ChangeRefusedError`.
   */
  const apply = useCallback(
    async (ops: ChangeOp[]): Promise<ChangeResult> => {
      const result = await applyChanges(projectId, contentChangeSet(ops));
      const outcome = readOutcome(result);
      if (outcome.status === "refused") throw new ChangeRefusedError(result, outcome.error);
      if (outcome.status === "stale") {
        throw new ChangeRefusedError(result, {
          code: "stale",
          message: outcome.op.error?.message ?? "the content changed",
        });
      }
      return result;
    },
    [applyChanges, projectId],
  );

  const staleDialog = useMemo<StaleDialogState | null>(() => {
    if (!pending) return null;
    const settle = (choice: StaleChoice) => {
      if (settled.current) return;
      settled.current = true;
      setPending(null);
      pending.resolve(choice);
    };
    return {
      ...pending.prompt,
      onReapply: () => settle("reapply"),
      onKeep: () => settle("keep"),
    };
  }, [pending]);

  return { commit, apply, staleDialog };
}

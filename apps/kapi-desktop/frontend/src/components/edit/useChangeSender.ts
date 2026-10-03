import { useCallback, useState } from "react";
import type { ChangeOp, ChangeResult, CurrentEdition, OpResult } from "@neokapi/contract-types";

import { type ChangeClient, rebaseOps, refusalMessage, staleRefusal } from "../../lib/changes";

/** A change that was refused because the content moved, held until the person decides. */
export interface PendingStale {
  ops: ChangeOp[];
  note?: string;
  op: OpResult;
  current: CurrentEdition;
}

export interface ChangeSenderOptions {
  /** Called with the result of a change set that landed. */
  onApplied?: (res: ChangeResult) => void | Promise<void>;
  /** Read the content again, after the person kept the text that changed. */
  onReload?: () => void | Promise<void>;
  /**
   * Rewrites an operation sent again over the current text, beyond taking its
   * revision: a check's fix finds its words again rather than trusting a run
   * range in text that moved.
   */
  rebase?: (op: ChangeOp) => ChangeOp;
}

export interface ChangeSender {
  /** Send a change set; resolves to its result, or null outside the app. */
  send: (ops: ChangeOp[], note?: string) => Promise<ChangeResult | null>;
  busy: boolean;
  /** A change refused because the content moved since it was read. */
  stale: PendingStale | null;
  /** Why the last change did not land, when it was not a moved revision. */
  error: string | null;
  /**
   * Send the held change again over the content as it stands, or `ops` in its
   * place: an editor's edits as they are now, typed after the refusal too.
   */
  reapply: (ops?: ChangeOp[]) => Promise<ChangeResult | null>;
  /** Drop the held change and read the content again. */
  discard: () => Promise<void>;
  /** Forget a refusal, as a new selection does. */
  clear: () => void;
}

/**
 * Sends change sets for one surface and holds what came back. A change that
 * lands calls onApplied. One refused because its revision moved is held with
 * the edition as it now stands, so the surface can show it and ask before
 * sending it again; any other refusal is an error sentence.
 */
export function useChangeSender(
  client: ChangeClient,
  opts: ChangeSenderOptions = {},
): ChangeSender {
  const { onApplied, onReload, rebase } = opts;
  const [busy, setBusy] = useState(false);
  const [stale, setStale] = useState<PendingStale | null>(null);
  const [error, setError] = useState<string | null>(null);

  const send = useCallback(
    async (ops: ChangeOp[], note?: string): Promise<ChangeResult | null> => {
      setBusy(true);
      setError(null);
      setStale(null);
      try {
        const res = await client.apply({ ops, ...(note ? { note } : {}) });
        if (!res) return null;
        if (res.status === "applied") {
          await onApplied?.(res);
          return res;
        }
        const moved = staleRefusal(res);
        if (moved) setStale({ ops, note, ...moved });
        else setError(refusalMessage(res));
        return res;
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        return null;
      } finally {
        setBusy(false);
      }
    },
    [client, onApplied],
  );

  const reapply = useCallback(
    async (ops?: ChangeOp[]) => {
      if (!stale) return null;
      return send(rebaseOps(ops ?? stale.ops, stale, rebase), stale.note);
    },
    [stale, send, rebase],
  );

  const discard = useCallback(async () => {
    setStale(null);
    setError(null);
    await onReload?.();
  }, [onReload]);

  const clear = useCallback(() => {
    setStale(null);
    setError(null);
  }, []);

  return { send, busy, stale, error, reapply, discard, clear };
}

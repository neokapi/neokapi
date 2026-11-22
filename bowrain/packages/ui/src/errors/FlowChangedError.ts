import type { FlowDefinitionInfo } from "../types/api";
import { ApiError } from "./ApiError";

/**
 * A flow save refused because someone saved the flow after it was read. It
 * carries the flow as it now stands, so the person can reload it or save their
 * version over it (a save based on current.revision).
 */
export class FlowChangedError extends Error {
  readonly current: FlowDefinitionInfo;

  constructor(current: FlowDefinitionInfo) {
    super("This flow was saved by someone else after you opened it.");
    this.name = "FlowChangedError";
    this.current = current;
  }
}

/**
 * The FlowChangedError err is or stands for: the error itself, or the server's
 * 409 answer to a stale save (`{code: "flow_changed", current}`). Null for any
 * other error.
 */
export function asFlowChanged(err: unknown): FlowChangedError | null {
  if (err instanceof FlowChangedError) return err;
  if (err instanceof ApiError && err.status === 409) {
    const body = err.body as { code?: unknown; current?: unknown } | undefined;
    if (body?.code === "flow_changed" && body.current && typeof body.current === "object") {
      return new FlowChangedError(body.current as FlowDefinitionInfo);
    }
  }
  return null;
}

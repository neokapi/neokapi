// Pure helper for reading a merge failure (AD-021). The REST adapter surfaces a
// non-2xx response as `Error("<status>: <body>")`; a 409 from the merge endpoint
// carries a JSON body of stale-draft conflicts. This unpacks that into the
// message + the OpConflict list the UI renders, with no React. Unit-tested.
import type { OpConflict } from "../../types/brand-graph";

export interface ParsedMergeError {
  message: string;
  conflicts: OpConflict[];
}

/** Extract a human message + any stale-draft conflicts from a merge error. */
export function parseMergeError(error: unknown): ParsedMergeError {
  if (!(error instanceof Error)) {
    return { message: "Merge failed.", conflicts: [] };
  }
  const m = error.message.match(/^(\d{3}):\s*([\s\S]*)$/);
  if (!m) return { message: error.message, conflicts: [] };
  try {
    const body = JSON.parse(m[2]) as { error?: string; conflicts?: OpConflict[] };
    return {
      message: body.error ?? error.message,
      conflicts: Array.isArray(body.conflicts) ? body.conflicts : [],
    };
  } catch {
    return { message: error.message, conflicts: [] };
  }
}

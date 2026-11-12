/**
 * Labels for the placeholders of a change-service read, taken from the runs a
 * surface already holds for the same block.
 *
 * A read lists a code's `equiv` and `disp` only where they differ from its
 * native form, and never the native form itself. A placeholder a code finder
 * marked, such as i18next's `{{name}}`, carries no label of its own, so the
 * editor would draw it with its vocabulary type ("code"). A surface that also
 * holds the block's runs, as the Review page does, gives such a placeholder its
 * display text as the label. Only the label changes: the edit text still names
 * the code by id, and a save sends the same tokens.
 */

import { projectRuns, type RunSpec } from "@neokapi/kapi-format";
import type { CodeRead, Run } from "@neokapi/contract-types";

/** One placeholder's token in edit text, and the text it reads as. */
type Label = readonly [token: string, label: string];

const LABELS: RunSpec<Run, Label> = {
  text: { dropped: "text carries no code" },
  ph: {
    expand: (r) => {
      const label = r.ph.disp || r.ph.data;
      return label ? [[`${r.ph.id}/`, label]] : [];
    },
  },
  pcOpen: { dropped: "a paired code's chip reads as its vocabulary tag" },
  pcClose: { dropped: "a paired code's chip reads as its vocabulary tag" },
  sub: { dropped: "a read labels a subblock reference by its id" },
  // A read lists the codes of every branch, so every branch is labelled.
  plural: {
    expand: (r) => Object.values(r.plural.forms).flatMap((form) => projectRuns(form, LABELS)),
  },
  select: {
    expand: (r) => Object.values(r.select.cases).flatMap((c) => projectRuns(c, LABELS)),
  },
  fallback: () => ["", ""],
};

/**
 * A placeholder only the vocabulary's generic chip could name: no label from
 * the read, and a type that says nothing more than "code". A typed placeholder
 * (a line break, an entity) keeps the chip its vocabulary entry draws.
 */
function unnamed(code: CodeRead): boolean {
  return (
    code.kind === "placeholder" &&
    !code.disp &&
    !code.equiv &&
    (!code.type || code.type === GENERIC_TYPE)
  );
}

/** The type a code finder gives every placeholder it marks. */
const GENERIC_TYPE = "code";

/**
 * `codes` with a `disp` on each unnamed placeholder, taken from
 * the first of `sequences` that holds a placeholder of the same id. Returns
 * `codes` itself when nothing is added.
 */
export function labelCodesFromRuns(
  codes: Readonly<Record<string, CodeRead>> | undefined,
  ...sequences: ReadonlyArray<readonly Run[] | undefined>
): Record<string, CodeRead> | undefined {
  if (!codes) return codes;
  let out: Record<string, CodeRead> | undefined;
  for (const runs of sequences) {
    for (const [token, label] of projectRuns(runs, LABELS)) {
      const code = (out ?? codes)[token];
      if (!token || !code || !unnamed(code)) continue;
      out ??= { ...codes };
      out[token] = { ...code, disp: label };
    }
  }
  return out ?? (codes as Record<string, CodeRead>);
}

// The rules the Fernwell sample keeps, each with who put it in force and
// when, as the Kept by list on the Rules screen reads them.

import type { KeptRule } from "../../types";

/** Kept by you, by a teammate, by kapi on the evidence, and by an assistant. */
export const KEPT_RULES: KeptRule[] = [
  {
    kind: "word",
    rule: "Write sign in, not login.",
    from: "terms",
    applies: "the whole project",
    kept_by: { kind: "person" },
    kept_at: "2026-10-28T14:10:00Z",
  },
  {
    kind: "name",
    rule: "Write Fernwell, not FernWell or Fern Well.",
    from: "terms",
    applies: "docs, app",
    kept_by: { kind: "person", name: "ada" },
    kept_at: "2026-10-20T09:00:00Z",
  },
  {
    kind: "word",
    rule: "Write member, not customer.",
    applies: "docs",
    kept_by: { kind: "tool", name: "kapi" },
    kept_at: "2026-10-26T08:30:00Z",
  },
  {
    kind: "note",
    rule: "Address the reader as you.",
    applies: "the whole project",
    kept_by: { kind: "agent", name: "claude", session: "s3" },
    kept_at: "2026-10-22T11:45:00Z",
  },
];

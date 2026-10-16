// The rules files kapi writes for the Fernwell project: written at the root
// and in one folder, and not written yet.

import type { RulesFileView, RulesFilesResult } from "../../types/api";

const ROOT_SECTION = `<!-- kapi:rules (written by kapi from this project's context; edit outside this section) -->
## Writing rules

These rules hold for every file in this project.

Voice: Fernwell help center. Plain, warm, second person (you).

Say this, not that:
- studio, not "business"
- sign in, not "log in"
- Fernwell: the product name. Never translate.
<!-- /kapi:rules -->`;

const DOCS_SECTION = `<!-- kapi:rules (written by kapi from this project's context; edit outside this section) -->
## Writing rules for docs/

Voice: Fernwell documentation. Precise, neutral, short sentences.

Say this, not that:
- select, not "click on"
<!-- /kapi:rules -->`;

/** The root's AGENTS.md and CLAUDE.md, and the docs folder's AGENTS.md, all written. */
export const RULES_FILES_WRITTEN: RulesFileView[] = [
  { path: "AGENTS.md", folder: "", section: ROOT_SECTION, exists: true },
  { path: "CLAUDE.md", folder: "", section: ROOT_SECTION, exists: true },
  { path: "docs/AGENTS.md", folder: "docs", section: DOCS_SECTION, exists: true },
];

/** The same files, before kapi has written any of them. */
export const RULES_FILES_NOT_WRITTEN: RulesFileView[] = RULES_FILES_WRITTEN.map((f) => ({
  ...f,
  exists: false,
}));

/** The root's files written, the docs folder's not yet. */
export const RULES_FILES_PARTLY_WRITTEN: RulesFileView[] = RULES_FILES_WRITTEN.map((f) => ({
  ...f,
  exists: f.folder === "",
}));

/** What writing them did. */
export const WRITE_RULES_RESULT: RulesFilesResult = {
  files: [
    { path: "AGENTS.md", action: "created" },
    { path: "CLAUDE.md", action: "created" },
    { path: "docs/AGENTS.md", action: "created" },
  ],
};

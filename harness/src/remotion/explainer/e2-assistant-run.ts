/**
 * The words beat 6 of E2 shows, copied verbatim from one recorded run of the
 * `e2-assistant-run` demo (a live Claude session with the kapi plugin in
 * harness/demos/e2-assistant-run/fixtures, captured 2026-10-07). Nothing here
 * is written for the video. Markdown is shown rendered: a `code` span is drawn
 * in the mono face without its backticks, and a wrapped line is the same text
 * broken to fit.
 *
 * Re-capturing that demo can change what the assistant writes; copy the new
 * words here from public/e2-assistant-run/capture.json and the sandbox
 * snapshot, field by field as noted, rather than editing them.
 */

/** help/getting-started.md as the fixture ships it (fixtures/help/getting-started.md, the paragraph). */
export const ARTICLE_BEFORE = [
  "To work together, create a Workspace for your",
  "team and invite people to it. Everything your",
  "team shares lives in its Workspace.",
];

/** The paragraph the assistant wrote (captures/e2-assistant-run/sandbox/help/getting-started.md). */
export const ARTICLE_AFTER = ["Create a Space for your team and invite", "people to it. Everything your team shares", "lives there."];

/** The word in ARTICLE_AFTER the rule is about, marked in the accent. */
export const ARTICLE_MARK = "Space";

/**
 * What kapi returned for the help article: the output of
 * `kapi context help/getting-started.md` (capture.json, first Bash tool result).
 * The heading, the rule's lead line, and the rule itself.
 */
export const CONTEXT_HEADING = "Writing help/getting-started.md";
export const CONTEXT_LEAD = "Say this, not that:";
export const CONTEXT_RULE = 'Space, not "Workspace"';

/** The assistant's explanation, one sentence of its final message (capture.json, the result event). */
export const REPLY = [
  { text: 'I also changed "Workspace" to "Space", because' },
  { text: "kapi's rule for the ", code: "help", after: " profile says" },
  { text: "customers only see the new name." },
];

/**
 * The check the assistant ran on its change: `kapi check --diff-against HEAD
 * --json` (capture.json), its `verdict` and `summary.findings` fields.
 */
export const CHECK = "verdict: passed · findings: 0";

/** The API page, from the same final message: the file it names, and the clause on why it kept the old word. */
export const API_FILE = "api/workspaces.md";
export const API_REPLY = { text: "the ", code: "api", after: " profile has no rename rule, so I kept it." };

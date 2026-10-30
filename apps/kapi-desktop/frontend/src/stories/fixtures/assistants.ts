// The Assistants page for Storybook and tests, shaped the way the backend
// renders one: nothing connected, Claude Code connected with sessions, and
// Claude Code connected while the check waits on the plugin install.

import type {
  AgentWiringResult,
  AssistantHostView,
  AssistantSession,
  AssistantsView,
  ConnectResult,
} from "../../types/api";

/** Claude Desktop's own configuration, under a home that resolves on no machine. */
export const CLAUDE_DESKTOP_CONFIG =
  "/fakehome/Library/Application Support/Claude/claude_desktop_config.json";

/** The two commands kapi connect prints for the kapi Claude Code plugin. */
export const PLUGIN_COMMANDS = [
  "/plugin marketplace add neokapi/claude-plugins",
  "/plugin install kapi@neokapi-plugins",
];

const host = (
  h: Partial<AssistantHostView> & { host: string; label: string },
): AssistantHostView => ({
  connected: false,
  check: false,
  available: true,
  ...h,
});

const OTHERS: AssistantHostView[] = [
  host({ host: "cursor", label: "Cursor" }),
  host({ host: "vscode", label: "VS Code" }),
  host({ host: "codex", label: "Codex" }),
  host({ host: "agents", label: "Agent skills" }),
  host({
    host: "claude-desktop",
    label: "Claude Desktop",
    personal: true,
    files: [CLAUDE_DESKTOP_CONFIG],
  }),
];

/** Claude Code with its files in place, and the check enabled or waiting. */
function claudeCode(check: boolean): AssistantHostView {
  return host({
    host: "claude-code",
    label: "Claude Code",
    connected: true,
    check,
    pending: check ? undefined : PLUGIN_COMMANDS,
    files: [".mcp.json", ".claude/skills/kapi"],
  });
}

/** One session, with the counts filled in. */
export function session(s: Partial<AssistantSession> & { id: string }): AssistantSession {
  return {
    actor: { kind: "agent", name: "claude", session: "sess-1", host: "studio" },
    last: "2026-10-29T14:20:00Z",
    quiet: true,
    operations: 1,
    suggestions: 0,
    corrections: 0,
    kept: 0,
    dropped: 0,
    awaiting: 0,
    ...s,
  };
}

/** An assistant's session with a suggestion open, and the person's own day. */
export const SESSIONS: AssistantSession[] = [
  session({
    id: "session:sess-1",
    operations: 3,
    suggestions: 2,
    corrections: 1,
    awaiting: 1,
  }),
  session({
    id: "actor:person/:2026-10-28",
    actor: { kind: "person" },
    last: "2026-10-28T09:05:00Z",
    operations: 2,
    kept: 2,
  }),
];

/** No assistant connected, and nothing recorded yet. */
export const ASSISTANTS_NONE: AssistantsView = {
  rules: 4,
  needs_checkout: false,
  project_key: "kapimart",
  hosts: [host({ host: "claude-code", label: "Claude Code", pending: PLUGIN_COMMANDS }), ...OTHERS],
  sessions: [],
};

/** Claude Code connected with the check on, and two sessions. */
export const ASSISTANTS_CLAUDE_CONNECTED: AssistantsView = {
  ...ASSISTANTS_NONE,
  hosts: [claudeCode(true), ...OTHERS],
  sessions: SESSIONS,
};

/** Claude Code connected, the check waiting on the plugin install. */
export const ASSISTANTS_CHECK_PENDING: AssistantsView = {
  ...ASSISTANTS_NONE,
  hosts: [claudeCode(false), ...OTHERS],
  sessions: SESSIONS.slice(0, 1),
};

/** Claude Code's own files in place, and the project's rules files deleted. */
export const ASSISTANTS_RULES_MISSING: AssistantsView = {
  ...ASSISTANTS_NONE,
  hosts: [{ ...claudeCode(true), connected: false, reason: "rules files missing" }, ...OTHERS],
  sessions: SESSIONS,
};

/** The assistants could not be read; the sessions were. */
export const ASSISTANTS_HOSTS_FAILED: AssistantsView = {
  ...ASSISTANTS_NONE,
  rules: undefined,
  hosts: [],
  hosts_error: "load project: kapi.yaml: yaml: line 3: did not find expected key",
  sessions: SESSIONS,
};

/** A tab over the project's context alone. */
export const ASSISTANTS_NO_CHECKOUT: AssistantsView = {
  rules: 0,
  needs_checkout: true,
  project_key: "kapimart",
  hosts: [],
  sessions: SESSIONS,
};

/** What connecting Cursor wrote. */
export const CONNECT_CURSOR_RESULT: ConnectResult = {
  root: "/fakehome/project",
  recipe: "/fakehome/project/kapi.yaml",
  dry_run: false,
  hosts: [
    {
      host: "cursor",
      label: "Cursor",
      connected: true,
      reads: ["rules", "MCP server", "skill"],
      files: [
        { path: ".cursor/mcp.json", kind: "mcp", action: "created" },
        { path: ".cursor/skills/kapi", kind: "skill", action: "kept" },
      ],
    },
  ],
  files: [
    { path: "kapi.yaml", kind: "recipe", action: "kept" },
    { path: "AGENTS.md", kind: "rules", action: "updated" },
  ],
};

/** What a second Connect reports: every file kept. */
export const CONNECT_NOTHING_RESULT: ConnectResult = {
  ...CONNECT_CURSOR_RESULT,
  hosts: [
    {
      ...CONNECT_CURSOR_RESULT.hosts[0],
      files: CONNECT_CURSOR_RESULT.hosts[0].files!.map((f) => ({ ...f, action: "kept" })),
    },
  ],
  files: CONNECT_CURSOR_RESULT.files.map((f) => ({ ...f, action: "kept" })),
};

/** What connecting Claude Desktop wrote. */
export const CONNECT_CLAUDE_DESKTOP_RESULT: AgentWiringResult = {
  hosts: ["claude-desktop"],
  files: [{ kind: "mcp", host: "claude-desktop", path: CLAUDE_DESKTOP_CONFIG, action: "updated" }],
};

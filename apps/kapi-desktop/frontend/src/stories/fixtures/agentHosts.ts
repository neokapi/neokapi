// The assistants kapi can connect for one project: every one connected, none
// connected, and Claude Desktop absent from this Mac.

import type { AgentHostStatus, AgentWiringResult } from "../../types/api";

/** Claude Desktop's own configuration, under a home that resolves on no machine. */
export const CLAUDE_DESKTOP_CONFIG =
  "/fakehome/Library/Application Support/Claude/claude_desktop_config.json";

const claudeCode = (wired: boolean): AgentHostStatus => ({
  host: "claude-code",
  label: "Claude Code",
  detected: true,
  wired,
  files: wired ? [".mcp.json", ".claude/skills/kapi"] : undefined,
});
const cursor = (wired: boolean): AgentHostStatus => ({
  host: "cursor",
  label: "Cursor",
  detected: true,
  wired,
  files: wired ? [".cursor/mcp.json"] : undefined,
});
const vscode = (wired: boolean): AgentHostStatus => ({
  host: "vscode",
  label: "VS Code",
  detected: true,
  wired,
  files: wired ? [".vscode/mcp.json"] : undefined,
});
const codex = (wired: boolean): AgentHostStatus => ({
  host: "codex",
  label: "Codex",
  detected: true,
  wired,
  files: wired ? [".codex/config.toml"] : undefined,
});
// The backend names Claude Desktop's configuration whether or not it is connected.
const claudeDesktop = (wired: boolean, detected = true): AgentHostStatus => ({
  host: "claude-desktop",
  label: "Claude Desktop",
  detected,
  wired,
  files: [CLAUDE_DESKTOP_CONFIG],
});

/** Every assistant on this Mac is connected to the project. */
export const HOSTS_ALL_CONNECTED: AgentHostStatus[] = [
  claudeCode(true),
  cursor(true),
  vscode(true),
  codex(true),
  claudeDesktop(true),
];

/** Every assistant is on this Mac and none is connected yet. */
export const HOSTS_NONE_CONNECTED: AgentHostStatus[] = [
  claudeCode(false),
  cursor(false),
  vscode(false),
  codex(false),
  claudeDesktop(false),
];

/**
 * Claude Code connected, Cursor on this Mac and not connected, and VS Code,
 * Codex and Claude Desktop neither installed nor connected.
 */
export const HOSTS_CLAUDE_DESKTOP_ABSENT: AgentHostStatus[] = [
  claudeCode(true),
  cursor(false),
  { ...vscode(false), detected: false },
  { ...codex(false), detected: false },
  claudeDesktop(false, false),
];

/** What connecting Cursor wrote. */
export const CONNECT_CURSOR_RESULT: AgentWiringResult = {
  hosts: ["cursor"],
  files: [{ kind: "mcp", host: "cursor", path: ".cursor/mcp.json", action: "created" }],
};

/** What connecting Claude Desktop wrote. */
export const CONNECT_CLAUDE_DESKTOP_RESULT: AgentWiringResult = {
  hosts: ["claude-desktop"],
  files: [{ kind: "mcp", host: "claude-desktop", path: CLAUDE_DESKTOP_CONFIG, action: "updated" }],
};

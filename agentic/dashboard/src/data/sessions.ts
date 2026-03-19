export interface ToolCall {
  tool: string;
  timestamp: string;
  durationMs: number;
  success: boolean;
}

export interface AgentSession {
  id: string;
  agentId: string;
  agentName: string;
  workspace: string;
  startTime: string;
  endTime: string;
  durationSecs: number;
  status: "succeeded" | "failed" | "running";
  toolCalls: ToolCall[];
  summary: string;
  costUsd: number;
}

const day = (daysAgo: number, hour: number, minute: number) => {
  const d = new Date();
  d.setDate(d.getDate() - daysAgo);
  d.setHours(hour, minute, 0, 0);
  return d.toISOString();
};

const endTime = (start: string, durationSecs: number) =>
  new Date(new Date(start).getTime() + durationSecs * 1000).toISOString();

function alexToolCalls(start: string): ToolCall[] {
  const base = new Date(start).getTime();
  return [
    { tool: "connector_pull", timestamp: new Date(base + 5_000).toISOString(), durationMs: 8200, success: true },
    { tool: "list_projects", timestamp: new Date(base + 15_000).toISOString(), durationMs: 1200, success: true },
    { tool: "connector_push", timestamp: new Date(base + 120_000).toISOString(), durationMs: 12400, success: true },
  ];
}

function sophieToolCalls(start: string, blockCount: number): ToolCall[] {
  const base = new Date(start).getTime();
  const calls: ToolCall[] = [];
  let offset = 3_000;
  for (let i = 0; i < blockCount; i++) {
    calls.push({ tool: "list_blocks", timestamp: new Date(base + offset).toISOString(), durationMs: 800, success: true });
    offset += 2_000;
    calls.push({ tool: "get_block", timestamp: new Date(base + offset).toISOString(), durationMs: 420, success: true });
    offset += 1_500;
    calls.push({ tool: "update_block", timestamp: new Date(base + offset).toISOString(), durationMs: 1800, success: true });
    offset += 3_000;
    if (i % 4 === 0) {
      calls.push({ tool: "tm_search", timestamp: new Date(base + offset).toISOString(), durationMs: 650, success: true });
      offset += 2_000;
    }
  }
  return calls;
}

function thomasToolCalls(start: string, blockCount: number): ToolCall[] {
  const base = new Date(start).getTime();
  const calls: ToolCall[] = [];
  let offset = 2_000;
  for (let i = 0; i < blockCount; i++) {
    calls.push({ tool: "list_blocks", timestamp: new Date(base + offset).toISOString(), durationMs: 750, success: true });
    offset += 1_800;
    calls.push({ tool: "get_block", timestamp: new Date(base + offset).toISOString(), durationMs: 380, success: true });
    offset += 1_200;
    calls.push({ tool: "update_block", timestamp: new Date(base + offset).toISOString(), durationMs: 1600, success: true });
    offset += 2_500;
    if (i % 5 === 0) {
      calls.push({ tool: "tm_search", timestamp: new Date(base + offset).toISOString(), durationMs: 580, success: true });
      offset += 1_500;
    }
  }
  return calls;
}

function meiToolCalls(start: string): ToolCall[] {
  const base = new Date(start).getTime();
  return [
    { tool: "list_blocks", timestamp: new Date(base + 2_000).toISOString(), durationMs: 900, success: true },
    { tool: "check_vocabulary", timestamp: new Date(base + 60_000).toISOString(), durationMs: 4200, success: true },
    { tool: "list_blocks", timestamp: new Date(base + 120_000).toISOString(), durationMs: 850, success: true },
    { tool: "check_vocabulary", timestamp: new Date(base + 180_000).toISOString(), durationMs: 3800, success: true },
    { tool: "run_flow", timestamp: new Date(base + 300_000).toISOString(), durationMs: 18500, success: true },
  ];
}

export const sessions: AgentSession[] = [
  // Today
  {
    id: "s01", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(0, 9, 0), endTime: endTime(day(0, 9, 0), 202), durationSecs: 202, status: "succeeded",
    toolCalls: alexToolCalls(day(0, 9, 0)),
    summary: "Pulled 14 new source blocks from excalidraw/excalidraw, pushed translated bundle.",
    costUsd: 0.08,
  },
  {
    id: "s02", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(0, 14, 0), endTime: endTime(day(0, 14, 0), 1334), durationSecs: 1334, status: "succeeded",
    toolCalls: sophieToolCalls(day(0, 14, 0), 24),
    summary: "Translated 24 blocks in fr-FR. 6 TM matches, 18 new translations.",
    costUsd: 1.82,
  },
  {
    id: "s03", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(0, 10, 0), endTime: endTime(day(0, 10, 0), 728), durationSecs: 728, status: "succeeded",
    toolCalls: meiToolCalls(day(0, 10, 0)),
    summary: "Reviewed fr-FR and de-DE blocks. 2 vocabulary warnings flagged.",
    costUsd: 0.65,
  },
  {
    id: "s04", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(0, 12, 0), endTime: endTime(day(0, 12, 0), 685), durationSecs: 685, status: "succeeded",
    toolCalls: meiToolCalls(day(0, 12, 0)),
    summary: "Reviewed recent translations. All checks passed.",
    costUsd: 0.58,
  },
  // Yesterday
  {
    id: "s05", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(1, 9, 0), endTime: endTime(day(1, 9, 0), 185), durationSecs: 185, status: "succeeded",
    toolCalls: alexToolCalls(day(1, 9, 0)),
    summary: "Morning sync: pulled 8 new blocks, pushed updated bundle.",
    costUsd: 0.07,
  },
  {
    id: "s06", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(1, 17, 0), endTime: endTime(day(1, 17, 0), 210), durationSecs: 210, status: "succeeded",
    toolCalls: alexToolCalls(day(1, 17, 0)),
    summary: "Evening sync: pushed 31 translated blocks to fork.",
    costUsd: 0.09,
  },
  {
    id: "s07", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(1, 14, 0), endTime: endTime(day(1, 14, 0), 1580), durationSecs: 1580, status: "succeeded",
    toolCalls: sophieToolCalls(day(1, 14, 0), 28),
    summary: "Translated 28 blocks in fr-FR. TM leverage 32%.",
    costUsd: 2.14,
  },
  {
    id: "s08", agentId: "thomas-weber", agentName: "Thomas Weber", workspace: "excalidraw",
    startTime: day(1, 14, 0), endTime: endTime(day(1, 14, 0), 1127), durationSecs: 1127, status: "succeeded",
    toolCalls: thomasToolCalls(day(1, 14, 0), 22),
    summary: "Translated 22 blocks in de-DE. Compound noun review applied.",
    costUsd: 1.68,
  },
  {
    id: "s09", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(1, 10, 0), endTime: endTime(day(1, 10, 0), 745), durationSecs: 745, status: "succeeded",
    toolCalls: meiToolCalls(day(1, 10, 0)),
    summary: "Review pass: 1 consistency issue in de-DE flagged.",
    costUsd: 0.62,
  },
  {
    id: "s10", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(1, 14, 30), endTime: endTime(day(1, 14, 30), 698), durationSecs: 698, status: "failed",
    toolCalls: [
      { tool: "list_blocks", timestamp: new Date(new Date(day(1, 14, 30)).getTime() + 2_000).toISOString(), durationMs: 920, success: true },
      { tool: "check_vocabulary", timestamp: new Date(new Date(day(1, 14, 30)).getTime() + 60_000).toISOString(), durationMs: 30000, success: false },
    ],
    summary: "Failed: MCP timeout on check_vocabulary — server did not respond within 30s.",
    costUsd: 0.31,
  },
  // 2 days ago
  {
    id: "s11", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(2, 9, 0), endTime: endTime(day(2, 9, 0), 198), durationSecs: 198, status: "succeeded",
    toolCalls: alexToolCalls(day(2, 9, 0)),
    summary: "Pulled 5 new blocks, no conflicts detected.",
    costUsd: 0.06,
  },
  {
    id: "s12", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(2, 14, 0), endTime: endTime(day(2, 14, 0), 1450), durationSecs: 1450, status: "succeeded",
    toolCalls: sophieToolCalls(day(2, 14, 0), 26),
    summary: "Translated 26 blocks in fr-FR. Added 4 terms to termbase.",
    costUsd: 1.95,
  },
  {
    id: "s13", agentId: "thomas-weber", agentName: "Thomas Weber", workspace: "excalidraw",
    startTime: day(2, 14, 0), endTime: endTime(day(2, 14, 0), 980), durationSecs: 980, status: "succeeded",
    toolCalls: thomasToolCalls(day(2, 14, 0), 18),
    summary: "Translated 18 blocks in de-DE. Umlaut consistency pass.",
    costUsd: 1.38,
  },
  {
    id: "s14", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(2, 16, 0), endTime: endTime(day(2, 16, 0), 712), durationSecs: 712, status: "succeeded",
    toolCalls: meiToolCalls(day(2, 16, 0)),
    summary: "All checks passed. Quality score stable.",
    costUsd: 0.55,
  },
  // 3 days ago
  {
    id: "s15", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(3, 9, 0), endTime: endTime(day(3, 9, 0), 175), durationSecs: 175, status: "succeeded",
    toolCalls: alexToolCalls(day(3, 9, 0)),
    summary: "Morning sync. 3 new source blocks pulled.",
    costUsd: 0.05,
  },
  {
    id: "s16", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(3, 17, 0), endTime: endTime(day(3, 17, 0), 195), durationSecs: 195, status: "failed",
    toolCalls: [
      { tool: "connector_pull", timestamp: new Date(new Date(day(3, 17, 0)).getTime() + 5_000).toISOString(), durationMs: 8200, success: true },
      { tool: "list_projects", timestamp: new Date(new Date(day(3, 17, 0)).getTime() + 15_000).toISOString(), durationMs: 1200, success: true },
      { tool: "connector_push", timestamp: new Date(new Date(day(3, 17, 0)).getTime() + 120_000).toISOString(), durationMs: 5000, success: false },
    ],
    summary: "Failed: connector_push — GitHub API rate limit exceeded. Retried next cycle.",
    costUsd: 0.04,
  },
  {
    id: "s17", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(3, 14, 0), endTime: endTime(day(3, 14, 0), 1680), durationSecs: 1680, status: "succeeded",
    toolCalls: sophieToolCalls(day(3, 14, 0), 30),
    summary: "Translated 30 blocks in fr-FR. Longest session this week.",
    costUsd: 2.35,
  },
  // 5 days ago
  {
    id: "s18", agentId: "thomas-weber", agentName: "Thomas Weber", workspace: "excalidraw",
    startTime: day(5, 14, 0), endTime: endTime(day(5, 14, 0), 1050), durationSecs: 1050, status: "succeeded",
    toolCalls: thomasToolCalls(day(5, 14, 0), 20),
    summary: "Translated 20 blocks in de-DE. Initial Excalidraw batch.",
    costUsd: 1.52,
  },
  {
    id: "s19", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(5, 16, 0), endTime: endTime(day(5, 16, 0), 690), durationSecs: 690, status: "succeeded",
    toolCalls: meiToolCalls(day(5, 16, 0)),
    summary: "Review pass on initial de-DE batch. 4 issues flagged.",
    costUsd: 0.58,
  },
  // 7 days ago
  {
    id: "s20", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(7, 9, 0), endTime: endTime(day(7, 9, 0), 240), durationSecs: 240, status: "succeeded",
    toolCalls: alexToolCalls(day(7, 9, 0)),
    summary: "Initial workspace setup. Pulled 142 source blocks from upstream.",
    costUsd: 0.12,
  },
  {
    id: "s21", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(7, 14, 0), endTime: endTime(day(7, 14, 0), 1820), durationSecs: 1820, status: "succeeded",
    toolCalls: sophieToolCalls(day(7, 14, 0), 30),
    summary: "First fr-FR pass: 30 blocks translated. Established terminology baseline.",
    costUsd: 2.42,
  },
  // 8 days ago
  {
    id: "s22", agentId: "thomas-weber", agentName: "Thomas Weber", workspace: "excalidraw",
    startTime: day(8, 14, 0), endTime: endTime(day(8, 14, 0), 1100), durationSecs: 1100, status: "succeeded",
    toolCalls: thomasToolCalls(day(8, 14, 0), 21),
    summary: "First de-DE pass: 21 blocks. Set up German terminology conventions.",
    costUsd: 1.62,
  },
  {
    id: "s23", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(8, 16, 0), endTime: endTime(day(8, 16, 0), 780), durationSecs: 780, status: "succeeded",
    toolCalls: meiToolCalls(day(8, 16, 0)),
    summary: "Initial review. Flagged 6 inconsistencies across fr-FR and de-DE.",
    costUsd: 0.68,
  },
  // 10 days ago
  {
    id: "s24", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(10, 9, 0), endTime: endTime(day(10, 9, 0), 310), durationSecs: 310, status: "succeeded",
    toolCalls: alexToolCalls(day(10, 9, 0)),
    summary: "Workspace bootstrap: configured connector, pulled initial source.",
    costUsd: 0.15,
  },
  // 12 days ago
  {
    id: "s25", agentId: "sophie-martin", agentName: "Sophie Martin", workspace: "excalidraw",
    startTime: day(12, 14, 0), endTime: endTime(day(12, 14, 0), 920), durationSecs: 920, status: "failed",
    toolCalls: [
      { tool: "list_blocks", timestamp: new Date(new Date(day(12, 14, 0)).getTime() + 3_000).toISOString(), durationMs: 800, success: true },
      { tool: "get_block", timestamp: new Date(new Date(day(12, 14, 0)).getTime() + 6_000).toISOString(), durationMs: 420, success: true },
      { tool: "update_block", timestamp: new Date(new Date(day(12, 14, 0)).getTime() + 10_000).toISOString(), durationMs: 45000, success: false },
    ],
    summary: "Failed: model error — GPT-4o returned malformed JSON response. Session aborted.",
    costUsd: 0.42,
  },
  {
    id: "s26", agentId: "mei-zhang", agentName: "Mei Zhang", workspace: "excalidraw",
    startTime: day(12, 10, 0), endTime: endTime(day(12, 10, 0), 650), durationSecs: 650, status: "succeeded",
    toolCalls: meiToolCalls(day(12, 10, 0)),
    summary: "Baseline review. All blocks checked, 0 issues.",
    costUsd: 0.52,
  },
  // 13 days ago
  {
    id: "s27", agentId: "alex-chen", agentName: "Alex Chen", workspace: "excalidraw",
    startTime: day(13, 17, 0), endTime: endTime(day(13, 17, 0), 188), durationSecs: 188, status: "succeeded",
    toolCalls: alexToolCalls(day(13, 17, 0)),
    summary: "Evening sync: pushed first translated batch to fork.",
    costUsd: 0.08,
  },
  {
    id: "s28", agentId: "thomas-weber", agentName: "Thomas Weber", workspace: "excalidraw",
    startTime: day(13, 14, 0), endTime: endTime(day(13, 14, 0), 1200), durationSecs: 1200, status: "succeeded",
    toolCalls: thomasToolCalls(day(13, 14, 0), 23),
    summary: "Translated 23 blocks in de-DE. Terminology setup session.",
    costUsd: 1.75,
  },
];

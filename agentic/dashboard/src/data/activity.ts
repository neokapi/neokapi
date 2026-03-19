export interface ActivityEntry {
  id: string;
  timestamp: Date;
  agentId: string;
  agentName: string;
  agentAvatar: string;
  accentColor: string;
  action: string;
  workspace: string;
  toolsUsed: string[];
}

const now = new Date();
const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000);
const hoursAgo = (h: number) => new Date(now.getTime() - h * 3_600_000);

export const activityFeed: ActivityEntry[] = [
  { id: "a1", timestamp: minutesAgo(12), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Pulled 14 new source blocks from excalidraw/excalidraw, pushed updated bundle to fork", workspace: "excalidraw", toolsUsed: ["connector_pull", "list_projects", "connector_push"] },
  { id: "a2", timestamp: minutesAgo(45), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Reviewed fr-FR and de-DE blocks. 2 vocabulary warnings flagged via check_vocabulary", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary", "run_flow"] },
  { id: "a3", timestamp: hoursAgo(1.5), agentId: "sophie-martin", agentName: "Sophie Martin", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Translated 24 blocks in fr-FR. 6 TM matches reused, 18 new translations", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a4", timestamp: hoursAgo(2), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Review pass completed. All recent translations passed quality checks", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary", "run_flow"] },
  { id: "a5", timestamp: hoursAgo(4), agentId: "thomas-weber", agentName: "Thomas Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 22 blocks in de-DE. Compound noun review applied to UI labels", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a6", timestamp: hoursAgo(5), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Evening sync: pushed 31 translated blocks to neokapi/agentic-excalidraw", workspace: "excalidraw", toolsUsed: ["connector_pull", "connector_push"] },
  { id: "a7", timestamp: hoursAgo(6), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Filed issue: inconsistent capitalization in de-DE toolbar labels", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary"] },
  { id: "a8", timestamp: hoursAgo(8), agentId: "sophie-martin", agentName: "Sophie Martin", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Translated 28 blocks in fr-FR. TM leverage at 32%", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a9", timestamp: hoursAgo(9), agentId: "thomas-weber", agentName: "Thomas Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 18 blocks in de-DE. Umlaut consistency pass completed", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block"] },
  { id: "a10", timestamp: hoursAgo(10), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Pulled 5 new blocks from upstream. No merge conflicts", workspace: "excalidraw", toolsUsed: ["connector_pull", "list_projects"] },
  { id: "a11", timestamp: hoursAgo(12), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Review failed: MCP timeout on check_vocabulary after 30s. Will retry next cycle", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary"] },
  { id: "a12", timestamp: hoursAgo(14), agentId: "sophie-martin", agentName: "Sophie Martin", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Translated 26 blocks in fr-FR. Added 4 new terms to termbase", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a13", timestamp: hoursAgo(16), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Push failed: GitHub API rate limit exceeded. Queued for retry", workspace: "excalidraw", toolsUsed: ["connector_pull", "list_projects", "connector_push"] },
  { id: "a14", timestamp: hoursAgo(18), agentId: "thomas-weber", agentName: "Thomas Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 20 blocks in de-DE. Initial Excalidraw batch complete", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a15", timestamp: hoursAgo(20), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Review pass on initial de-DE batch. 4 issues flagged and filed", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary", "run_flow"] },
  { id: "a16", timestamp: hoursAgo(24), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Initial workspace setup. Pulled 142 source blocks from excalidraw/excalidraw", workspace: "excalidraw", toolsUsed: ["connector_pull", "list_projects"] },
  { id: "a17", timestamp: hoursAgo(28), agentId: "sophie-martin", agentName: "Sophie Martin", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "First fr-FR pass: 30 blocks translated. Established terminology baseline", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block", "tm_search"] },
  { id: "a18", timestamp: hoursAgo(32), agentId: "thomas-weber", agentName: "Thomas Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "First de-DE pass: 21 blocks. Set up German terminology conventions", workspace: "excalidraw", toolsUsed: ["list_blocks", "get_block", "update_block"] },
  { id: "a19", timestamp: hoursAgo(36), agentId: "mei-zhang", agentName: "Mei Zhang", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Initial review. Flagged 6 inconsistencies across fr-FR and de-DE", workspace: "excalidraw", toolsUsed: ["list_blocks", "check_vocabulary", "run_flow"] },
  { id: "a20", timestamp: hoursAgo(48), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Workspace bootstrap: configured connector for excalidraw/excalidraw", workspace: "excalidraw", toolsUsed: ["connector_pull"] },
];

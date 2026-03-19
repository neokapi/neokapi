export interface MemoryEntry {
  id: string;
  timestamp: Date;
  agentId: string;
  agentName: string;
  summary: string;
  additions: number;
  deletions: number;
}

const now = new Date();
const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000);
const hoursAgo = (h: number) => new Date(now.getTime() - h * 3_600_000);

export const memoryLog: MemoryEntry[] = [
  {
    id: "m1",
    timestamp: hoursAgo(1.5),
    agentId: "sophie-martin",
    agentName: "Sophie Martin",
    summary: "+3 TM entries, updated French terminology notes for UI labels",
    additions: 12,
    deletions: 2,
  },
  {
    id: "m2",
    timestamp: hoursAgo(2),
    agentId: "mei-zhang",
    agentName: "Mei Zhang",
    summary: "Updated quality checklist: added placeholder validation rule",
    additions: 5,
    deletions: 0,
  },
  {
    id: "m3",
    timestamp: hoursAgo(4),
    agentId: "thomas-weber",
    agentName: "Thomas Weber",
    summary: "Added compound noun splitting rules for German toolbar labels",
    additions: 8,
    deletions: 1,
  },
  {
    id: "m4",
    timestamp: hoursAgo(5),
    agentId: "alex-chen",
    agentName: "Alex Chen",
    summary: "Updated connector config: increased push retry count to 3",
    additions: 3,
    deletions: 1,
  },
  {
    id: "m5",
    timestamp: hoursAgo(8),
    agentId: "sophie-martin",
    agentName: "Sophie Martin",
    summary: "+5 TM entries from batch translation. Noted 'canvas' ambiguity in French",
    additions: 18,
    deletions: 0,
  },
  {
    id: "m6",
    timestamp: hoursAgo(10),
    agentId: "mei-zhang",
    agentName: "Mei Zhang",
    summary: "Flagged capitalization pattern: German nouns must be capitalized in menu items",
    additions: 4,
    deletions: 0,
  },
  {
    id: "m7",
    timestamp: hoursAgo(14),
    agentId: "thomas-weber",
    agentName: "Thomas Weber",
    summary: "Updated umlaut consistency notes for de-DE shape names",
    additions: 6,
    deletions: 3,
  },
  {
    id: "m8",
    timestamp: hoursAgo(24),
    agentId: "alex-chen",
    agentName: "Alex Chen",
    summary: "First session -- initialized workspace memory, set up connector mappings",
    additions: 42,
    deletions: 0,
  },
  {
    id: "m9",
    timestamp: hoursAgo(28),
    agentId: "sophie-martin",
    agentName: "Sophie Martin",
    summary: "Established French terminology baseline: 30 core terms defined",
    additions: 65,
    deletions: 0,
  },
  {
    id: "m10",
    timestamp: hoursAgo(32),
    agentId: "thomas-weber",
    agentName: "Thomas Weber",
    summary: "Set up German terminology conventions: formal register, compound noun rules",
    additions: 48,
    deletions: 0,
  },
  {
    id: "m11",
    timestamp: minutesAgo(12),
    agentId: "alex-chen",
    agentName: "Alex Chen",
    summary: "Noted 14 new source blocks pulled; updated sync tracking metadata",
    additions: 7,
    deletions: 2,
  },
  {
    id: "m12",
    timestamp: minutesAgo(45),
    agentId: "mei-zhang",
    agentName: "Mei Zhang",
    summary: "Logged 2 vocabulary warnings; updated known-issues reference",
    additions: 4,
    deletions: 0,
  },
];

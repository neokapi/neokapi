export interface Agent {
  id: string;
  name: string;
  role: string;
  title: string;
  model: string;
  schedule: string;
  targetLanguage?: string;
  accentColor: string;
  personality: string[];
  status: "active" | "idle" | "sleeping";
  avatar: string;
  workspace: string;
  lastSession: {
    time: string;
    duration: string;
    status: "succeeded" | "failed";
  };
  stats: {
    sessionsThisWeek: number;
    toolCallsToday: number;
    lastActive: string;
    issuesFiled: number;
  };
}

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

export const agents: Agent[] = [
  {
    id: "alex-chen",
    name: "Alex Chen",
    role: "L10N Engineer",
    title: "L10N Engineer",
    model: "GPT-4o-mini",
    schedule: "Weekdays 9am & 5pm",
    accentColor: "amber",
    personality: ["Methodical", "CLI-first", "Ship it"],
    status: "active",
    avatar: "\u{1F6E0}\u{FE0F}",
    workspace: "excalidraw",
    lastSession: {
      time: minutesAgo(12),
      duration: "3m 22s",
      status: "succeeded",
    },
    stats: {
      sessionsThisWeek: 14,
      toolCallsToday: 18,
      lastActive: minutesAgo(12),
      issuesFiled: 2,
    },
  },
  {
    id: "sophie-martin",
    name: "Sophie Martin",
    role: "French Language Expert",
    title: "French Language Expert",
    model: "GPT-4o",
    schedule: "Weekdays 2pm",
    targetLanguage: "fr-FR",
    accentColor: "blue",
    personality: ["Nuanced", "Termbase-focused", "Thorough"],
    status: "active",
    avatar: "\u{1F1EB}\u{1F1F7}",
    workspace: "excalidraw",
    lastSession: {
      time: hoursAgo(1.5),
      duration: "22m 14s",
      status: "succeeded",
    },
    stats: {
      sessionsThisWeek: 5,
      toolCallsToday: 67,
      lastActive: hoursAgo(1.5),
      issuesFiled: 1,
    },
  },
  {
    id: "thomas-weber",
    name: "Thomas Weber",
    role: "German Language Expert",
    title: "German Language Expert",
    model: "GPT-4o",
    schedule: "Weekdays 2pm",
    targetLanguage: "de-DE",
    accentColor: "rose",
    personality: ["Precision-focused", "Compound-noun expert", "Concise"],
    status: "sleeping",
    avatar: "\u{1F1E9}\u{1F1EA}",
    workspace: "excalidraw",
    lastSession: {
      time: hoursAgo(4),
      duration: "18m 47s",
      status: "succeeded",
    },
    stats: {
      sessionsThisWeek: 5,
      toolCallsToday: 0,
      lastActive: hoursAgo(4),
      issuesFiled: 3,
    },
  },
  {
    id: "mei-zhang",
    name: "Mei Zhang",
    role: "Reviewer",
    title: "Reviewer",
    model: "GPT-4o",
    schedule: "Every 2 hours",
    accentColor: "teal",
    personality: ["Quality gate", "Cross-lingual", "Strict"],
    status: "active",
    avatar: "\u{1F50D}",
    workspace: "excalidraw",
    lastSession: {
      time: minutesAgo(45),
      duration: "12m 08s",
      status: "succeeded",
    },
    stats: {
      sessionsThisWeek: 21,
      toolCallsToday: 34,
      lastActive: minutesAgo(45),
      issuesFiled: 5,
    },
  },
];

export const accentColorMap: Record<string, string> = {
  amber: "#f59e0b",
  emerald: "#10b981",
  blue: "#3b82f6",
  rose: "#f43f5e",
  violet: "#8b5cf6",
  slate: "#94a3b8",
  teal: "#14b8a6",
};

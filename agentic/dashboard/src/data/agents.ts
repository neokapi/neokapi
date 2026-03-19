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
  stats: {
    blocksProcessed: number;
    tmEntries: number;
    issuesFiled: number;
    sessionsCompleted: number;
  };
  avatar: string;
}

export const agents: Agent[] = [
  {
    id: "alex-chen",
    name: "Alex Chen",
    role: "Developer",
    title: "Senior DevOps Engineer",
    model: "GPT-4o-mini",
    schedule: "Weekdays 9am & 5pm",
    accentColor: "amber",
    personality: ["Methodical", "CLI-first", "Ship it"],
    status: "active",
    stats: {
      blocksProcessed: 4218,
      tmEntries: 0,
      issuesFiled: 12,
      sessionsCompleted: 187,
    },
    avatar: "\u{1F6E0}\u{FE0F}",
  },
  {
    id: "maria-santos",
    name: "Maria Santos",
    role: "Brand Manager",
    title: "Localization Brand Specialist",
    model: "Claude Sonnet 4.5",
    schedule: "MWF 10am",
    accentColor: "emerald",
    personality: ["Detail-oriented", "Brand guardian", "Opinionated"],
    status: "idle",
    stats: {
      blocksProcessed: 1842,
      tmEntries: 312,
      issuesFiled: 28,
      sessionsCompleted: 64,
    },
    avatar: "\u{1F3A8}",
  },
  {
    id: "jean-pierre-dubois",
    name: "Jean-Pierre Dubois",
    role: "French Translator",
    title: "Senior French Linguist",
    model: "Claude Sonnet 4.5",
    schedule: "Weekdays 2pm",
    targetLanguage: "fr-FR",
    accentColor: "blue",
    personality: ["Formal register", "Termbase-obsessed", "60% accept rate"],
    status: "active",
    stats: {
      blocksProcessed: 3156,
      tmEntries: 2847,
      issuesFiled: 9,
      sessionsCompleted: 142,
    },
    avatar: "\u{1F1EB}\u{1F1F7}",
  },
  {
    id: "katrin-weber",
    name: "Katrin Weber",
    role: "German Translator",
    title: "Technical German Linguist",
    model: "Claude Sonnet 4.5",
    schedule: "Weekdays 2pm",
    targetLanguage: "de-DE",
    accentColor: "rose",
    personality: ["Precision-focused", "Engineering mind", "40% accept rate"],
    status: "sleeping",
    stats: {
      blocksProcessed: 2891,
      tmEntries: 2534,
      issuesFiled: 15,
      sessionsCompleted: 128,
    },
    avatar: "\u{1F1E9}\u{1F1EA}",
  },
  {
    id: "yuki-tanaka",
    name: "Yuki Tanaka",
    role: "Japanese Translator",
    title: "CJK Localization Expert",
    model: "Claude Sonnet 4.5",
    schedule: "Weekdays 8pm",
    targetLanguage: "ja-JP",
    accentColor: "violet",
    personality: ["CJK specialist", "UX-aware", "30% accept rate"],
    status: "sleeping",
    stats: {
      blocksProcessed: 2104,
      tmEntries: 1876,
      issuesFiled: 22,
      sessionsCompleted: 96,
    },
    avatar: "\u{1F1EF}\u{1F1F5}",
  },
  {
    id: "lisa-chen",
    name: "Lisa Chen",
    role: "Project Manager",
    title: "Localization Program Manager",
    model: "GPT-4o",
    schedule: "Weekdays 10am",
    accentColor: "slate",
    personality: ["Metrics-driven", "Deadline-focused", "Weekly reports"],
    status: "idle",
    stats: {
      blocksProcessed: 0,
      tmEntries: 0,
      issuesFiled: 34,
      sessionsCompleted: 52,
    },
    avatar: "\u{1F4CA}",
  },
  {
    id: "taylor-kim",
    name: "Taylor Kim",
    role: "QA Engineer",
    title: "Localization QA Specialist",
    model: "GPT-4o",
    schedule: "Every 2 hours",
    accentColor: "teal",
    personality: ["Automated QA", "Placeholder hawk", "Zero tolerance"],
    status: "active",
    stats: {
      blocksProcessed: 5672,
      tmEntries: 0,
      issuesFiled: 47,
      sessionsCompleted: 312,
    },
    avatar: "\u{1F50D}",
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

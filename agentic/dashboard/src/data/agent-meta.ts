/**
 * Static agent persona metadata.
 *
 * These are the display attributes that do NOT come from the Bowrain API
 * (the API provides user_id, role, joined_at via the members endpoint).
 * We merge this metadata with live API data in ApiContext.
 */

export interface AgentMeta {
  userId: string; // matches API user_id (e.g., "agent-alex")
  displayName: string;
  role: string;
  model: string;
  schedule: string;
  targetLanguage?: string;
  accentColor: string;
  personality: string[];
  avatar: string; // emoji
}

export const agentMeta: AgentMeta[] = [
  {
    userId: "agent-alex",
    displayName: "Alex Chen",
    role: "L10N Engineer",
    model: "GPT-4o-mini",
    schedule: "Weekdays 9am & 5pm",
    accentColor: "amber",
    personality: ["Methodical", "CLI-first", "Ship it"],
    avatar: "\u{1F6E0}\uFE0F",
  },
  {
    userId: "agent-sophie",
    displayName: "Sophie Martin",
    role: "French Language Expert",
    model: "GPT-4o",
    schedule: "Weekdays 2pm",
    targetLanguage: "fr-FR",
    accentColor: "blue",
    personality: ["Nuanced", "Termbase-focused", "Thorough"],
    avatar: "\u{1F1EB}\u{1F1F7}",
  },
  {
    userId: "agent-thomas",
    displayName: "Thomas Weber",
    role: "German Language Expert",
    model: "GPT-4o",
    schedule: "Weekdays 2pm",
    targetLanguage: "de-DE",
    accentColor: "rose",
    personality: ["Precision-focused", "Compound-noun expert", "Concise"],
    avatar: "\u{1F1E9}\u{1F1EA}",
  },
  {
    userId: "agent-mei",
    displayName: "Mei Zhang",
    role: "Reviewer",
    model: "GPT-4o",
    schedule: "Every 2 hours",
    accentColor: "teal",
    personality: ["Quality gate", "Cross-lingual", "Strict"],
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

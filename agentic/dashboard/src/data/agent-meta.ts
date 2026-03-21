/**
 * Static agent persona metadata.
 *
 * These are the display attributes that do NOT come from the Bowrain API
 * (the API provides user_id, role, joined_at via the members endpoint).
 * We merge this metadata with live API data in ApiContext.
 */

export interface AgentMeta {
  userId: string; // matches API user_id (e.g., "agent-coordinator")
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
    userId: "agent-coordinator",
    displayName: "Coordinator",
    role: "Fleet Coordinator",
    model: "GPT-5",
    schedule: "Weekdays 8:00 UTC",
    accentColor: "violet",
    personality: ["Strategic", "Observant", "Reports via GitHub issues"],
    avatar: "\u{1F9ED}",
  },
  {
    userId: "agent-maria",
    displayName: "Maria",
    role: "French Translator",
    model: "GPT-5-mini",
    schedule: "Mon/Wed/Fri 10:00 UTC",
    targetLanguage: "fr-FR",
    accentColor: "blue",
    personality: ["Nuanced", "Termbase-focused", "Thorough"],
    avatar: "\u{1F1EB}\u{1F1F7}",
  },
  {
    userId: "agent-katrin",
    displayName: "Katrin",
    role: "German Translator",
    model: "GPT-5-mini",
    schedule: "Tue/Thu 10:00 UTC",
    targetLanguage: "de-DE",
    accentColor: "rose",
    personality: ["Precision-focused", "Compound-noun expert", "Concise"],
    avatar: "\u{1F1E9}\u{1F1EA}",
  },
  {
    userId: "agent-yuki",
    displayName: "Yuki",
    role: "Japanese Translator",
    model: "GPT-5-mini",
    schedule: "Mon/Wed/Fri 11:00 UTC",
    targetLanguage: "ja-JP",
    accentColor: "teal",
    personality: ["Culturally aware", "Honorifics expert", "Meticulous"],
    avatar: "\u{1F1EF}\u{1F1F5}",
  },
  {
    userId: "agent-alex",
    displayName: "Alex",
    role: "Quality Reviewer",
    model: "GPT-5-mini",
    schedule: "Tue/Thu/Sat 14:00 UTC",
    accentColor: "amber",
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

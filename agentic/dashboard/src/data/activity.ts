export interface ActivityEntry {
  id: string;
  timestamp: Date;
  agentId: string;
  agentName: string;
  agentAvatar: string;
  accentColor: string;
  action: string;
}

const now = new Date();
const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000);
const hoursAgo = (h: number) => new Date(now.getTime() - h * 3_600_000);

export const activityFeed: ActivityEntry[] = [
  { id: "a1", timestamp: minutesAgo(2), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "QA pass on Capacitor fr-FR: 98.2% score, 3 warnings" },
  { id: "a2", timestamp: minutesAgo(8), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Pushed translated bundle to bowrain-test/capacitor#l10n-sync" },
  { id: "a3", timestamp: minutesAgo(15), agentId: "jean-pierre-dubois", agentName: "Jean-Pierre Dubois", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Translated 47 blocks in Capacitor fr-FR (32 from TM, 15 new)" },
  { id: "a4", timestamp: minutesAgo(22), agentId: "maria-santos", agentName: "Maria Santos", agentAvatar: "\u{1F3A8}", accentColor: "emerald", action: "Updated brand terminology: 'deployment' \u2192 'd\u00e9ploiement' (not 'mise en service')" },
  { id: "a5", timestamp: minutesAgo(35), agentId: "lisa-chen", agentName: "Lisa Chen", agentAvatar: "\u{1F4CA}", accentColor: "slate", action: "Weekly report: 312 blocks translated, TM reuse up 4.2%" },
  { id: "a6", timestamp: minutesAgo(48), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Filed issue: untranslated placeholder {count} in Flipt de-DE" },
  { id: "a7", timestamp: hoursAgo(1), agentId: "katrin-weber", agentName: "Katrin Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 38 blocks in Flipt de-DE (rejected 23 AI suggestions)" },
  { id: "a8", timestamp: hoursAgo(1.3), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Pulled upstream changes from flipt-io/flipt (12 new source blocks)" },
  { id: "a9", timestamp: hoursAgo(1.5), agentId: "jean-pierre-dubois", agentName: "Jean-Pierre Dubois", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Added 18 entries to French termbase from Listmonk glossary" },
  { id: "a10", timestamp: hoursAgo(2), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "QA pass on Listmonk ja-JP: 95.1% score, 7 warnings, 1 error" },
  { id: "a11", timestamp: hoursAgo(2.2), agentId: "yuki-tanaka", agentName: "Yuki Tanaka", agentAvatar: "\u{1F1EF}\u{1F1F5}", accentColor: "violet", action: "Translated 29 blocks in Listmonk ja-JP (CJK spacing fixes applied)" },
  { id: "a12", timestamp: hoursAgo(2.5), agentId: "maria-santos", agentName: "Maria Santos", agentAvatar: "\u{1F3A8}", accentColor: "emerald", action: "Flagged inconsistent capitalization in Infisical en-US source" },
  { id: "a13", timestamp: hoursAgo(3), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Created PR #47 on bowrain-test/listmonk with fr-FR translations" },
  { id: "a14", timestamp: hoursAgo(3.5), agentId: "katrin-weber", agentName: "Katrin Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 52 blocks in Infisical de-DE (compound noun review)" },
  { id: "a15", timestamp: hoursAgo(4), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "QA pass on Infisical fr-FR: 96.8% score, 5 warnings" },
  { id: "a16", timestamp: hoursAgo(4.5), agentId: "lisa-chen", agentName: "Lisa Chen", agentAvatar: "\u{1F4CA}", accentColor: "slate", action: "Assigned Infisical ja-JP backlog to Yuki \u2014 1,504 blocks remaining" },
  { id: "a17", timestamp: hoursAgo(5), agentId: "jean-pierre-dubois", agentName: "Jean-Pierre Dubois", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Translated 63 blocks in Infisical fr-FR (TM leverage: 42%)" },
  { id: "a18", timestamp: hoursAgo(5.5), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "bowrain pull on Infisical \u2014 synced 89 new source blocks" },
  { id: "a19", timestamp: hoursAgo(6), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "Filed issue: truncated string in Capacitor ja-JP settings.title" },
  { id: "a20", timestamp: hoursAgo(6.5), agentId: "yuki-tanaka", agentName: "Yuki Tanaka", agentAvatar: "\u{1F1EF}\u{1F1F5}", accentColor: "violet", action: "Fixed CJK line-breaking issue in Capacitor ja-JP nav labels" },
  { id: "a21", timestamp: hoursAgo(7), agentId: "maria-santos", agentName: "Maria Santos", agentAvatar: "\u{1F3A8}", accentColor: "emerald", action: "Approved 8 new brand terms for German localization guide" },
  { id: "a22", timestamp: hoursAgo(8), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Merged PR #45 on bowrain-test/flipt (de-DE batch)" },
  { id: "a23", timestamp: hoursAgo(9), agentId: "katrin-weber", agentName: "Katrin Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Translated 44 blocks in Capacitor de-DE (Umlaut consistency pass)" },
  { id: "a24", timestamp: hoursAgo(10), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "QA sweep complete: 4 projects, 12 locales checked, 14 issues found" },
  { id: "a25", timestamp: hoursAgo(11), agentId: "jean-pierre-dubois", agentName: "Jean-Pierre Dubois", agentAvatar: "\u{1F1EB}\u{1F1F7}", accentColor: "blue", action: "Reviewed and accepted 12 TM suggestions for Flipt fr-FR" },
  { id: "a26", timestamp: hoursAgo(12), agentId: "lisa-chen", agentName: "Lisa Chen", agentAvatar: "\u{1F4CA}", accentColor: "slate", action: "Updated project timeline: Capacitor fr-FR on track for 100% by W14" },
  { id: "a27", timestamp: hoursAgo(14), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "Configured bowrain sync for Infisical \u2014 watching src/locales/**/*.json" },
  { id: "a28", timestamp: hoursAgo(16), agentId: "yuki-tanaka", agentName: "Yuki Tanaka", agentAvatar: "\u{1F1EF}\u{1F1F5}", accentColor: "violet", action: "Translated 35 blocks in Flipt ja-JP (honorific level: polite)" },
  { id: "a29", timestamp: hoursAgo(18), agentId: "maria-santos", agentName: "Maria Santos", agentAvatar: "\u{1F3A8}", accentColor: "emerald", action: "Created style guide addendum for Japanese keigo conventions" },
  { id: "a30", timestamp: hoursAgo(20), agentId: "taylor-kim", agentName: "Taylor Kim", agentAvatar: "\u{1F50D}", accentColor: "teal", action: "QA regression test: all previously filed issues verified as fixed" },
  { id: "a31", timestamp: hoursAgo(22), agentId: "alex-chen", agentName: "Alex Chen", agentAvatar: "\u{1F6E0}\u{FE0F}", accentColor: "amber", action: "bowrain push \u2014 deployed Listmonk translations to staging" },
  { id: "a32", timestamp: hoursAgo(24), agentId: "katrin-weber", agentName: "Katrin Weber", agentAvatar: "\u{1F1E9}\u{1F1EA}", accentColor: "rose", action: "Completed Listmonk de-DE initial pass \u2014 535/645 blocks done" },
];

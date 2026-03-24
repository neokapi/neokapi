const BASE = "/api/v1/pulse";

export interface PulseOverview {
  workspace: PulseWorkspaceInfo;
  projects: PulseProjectSummary[];
  top_languages: PulseLanguageRank[];
  top_contributors: PulseContributor[];
  rising_stars: PulseRisingStar[];
  recent_activity: PulseActivity[];
  stats: PulseGlobalStats;
}

export interface PulseWorkspaceInfo {
  name: string;
  slug: string;
  description: string;
  logo_url: string;
}

export interface PulseGlobalStats {
  total_projects: number;
  total_languages: number;
  total_contributors: number;
  total_words: number;
  translated_words: number;
  overall_percent: number;
}

export interface PulseProjectSummary {
  id: string;
  name: string;
  source_language: string;
  target_languages: string[];
  total_words: number;
  translated_words: number;
  percentage: number;
  locales: LocaleTranslationStats[];
}

export interface LocaleTranslationStats {
  locale: string;
  translated_blocks: number;
  total_blocks: number;
  translated_words: number;
  total_words: number;
  percentage: number;
}

export interface PulseLanguageRank {
  locale: string;
  translated_words: number;
  total_words: number;
  percentage: number;
  contributors: number;
  recent_activity: number;
}

export interface PulseContributor {
  name: string;
  avatar_url?: string;
  translations: number;
  reviews: number;
  languages: string[];
}

export interface PulseRisingStar {
  name: string;
  type: "user" | "language" | "project";
  growth: number;
  current: number;
  previous: number;
}

export interface PulseActivity {
  id: string;
  type: string;
  actor: string;
  avatar_url?: string;
  project: string;
  locale?: string;
  summary: string;
  timestamp: string;
}

export interface PulseProjectDetail {
  project: PulseProjectSummary;
  locales: LocaleTranslationStats[];
  items: ItemTranslationStats[];
}

export interface ItemTranslationStats {
  item_name: string;
  item_id: string;
  format: string;
  block_count: number;
  word_count: number;
  locales: LocaleTranslationStats[];
}

export interface PulseLeaderboard {
  contributors: PulseContributor[];
  languages: PulseLanguageRank[];
}

export interface PulseTermEntry {
  id: string;
  term: string;
  definition: string;
  domain?: string;
  locale: string;
  translations?: Record<string, string>;
}

export async function fetchOverview(workspace: string): Promise<PulseOverview> {
  const res = await fetch(`${BASE}/${workspace}`);
  if (!res.ok) throw new Error(`Failed to fetch overview: ${res.status}`);
  return res.json();
}

export async function fetchProjects(workspace: string): Promise<{ projects: PulseProjectSummary[] }> {
  const res = await fetch(`${BASE}/${workspace}/projects`);
  if (!res.ok) throw new Error(`Failed to fetch projects: ${res.status}`);
  return res.json();
}

export async function fetchProjectDetail(workspace: string, pid: string): Promise<PulseProjectDetail> {
  const res = await fetch(`${BASE}/${workspace}/projects/${pid}`);
  if (!res.ok) throw new Error(`Failed to fetch project: ${res.status}`);
  return res.json();
}

export async function fetchLocaleDetail(workspace: string, pid: string, locale: string): Promise<unknown> {
  const res = await fetch(`${BASE}/${workspace}/projects/${pid}/lang/${locale}`);
  if (!res.ok) throw new Error(`Failed to fetch locale: ${res.status}`);
  return res.json();
}

export async function fetchActivity(workspace: string, params?: URLSearchParams): Promise<{ activities: PulseActivity[] }> {
  const qs = params ? `?${params.toString()}` : "";
  const res = await fetch(`${BASE}/${workspace}/activity${qs}`);
  if (!res.ok) throw new Error(`Failed to fetch activity: ${res.status}`);
  return res.json();
}

export async function fetchLeaderboard(workspace: string, params?: URLSearchParams): Promise<PulseLeaderboard> {
  const qs = params ? `?${params.toString()}` : "";
  const res = await fetch(`${BASE}/${workspace}/leaderboard${qs}`);
  if (!res.ok) throw new Error(`Failed to fetch leaderboard: ${res.status}`);
  return res.json();
}

export async function fetchTerms(workspace: string, params?: URLSearchParams): Promise<{ terms: PulseTermEntry[] }> {
  const qs = params ? `?${params.toString()}` : "";
  const res = await fetch(`${BASE}/${workspace}/terms${qs}`);
  if (!res.ok) throw new Error(`Failed to fetch terms: ${res.status}`);
  return res.json();
}

export const mockStats = {
  total_projects: 3,
  total_languages: 5,
  total_contributors: 12,
  total_words: 45000,
  translated_words: 32000,
  overall_percent: 71.1,
};

export const mockProjects = [
  {
    id: "p-1",
    name: "Web Application",
    source_language: "en-US",
    target_languages: ["fr-FR", "de-DE", "ja-JP"],
    total_words: 20000,
    translated_words: 15000,
    percentage: 75.0,
  },
  {
    id: "p-2",
    name: "Mobile App",
    source_language: "en-US",
    target_languages: ["es-ES", "pt-BR"],
    total_words: 15000,
    translated_words: 12000,
    percentage: 80.0,
  },
  {
    id: "p-3",
    name: "Documentation",
    source_language: "en-US",
    target_languages: ["fr-FR", "zh-CN"],
    total_words: 10000,
    translated_words: 5000,
    percentage: 50.0,
  },
];

export const mockLanguages = [
  { locale: "fr-FR", translated_words: 18000, total_words: 22000, percentage: 81.8, contributors: 4, recent_activity: 45 },
  { locale: "de-DE", translated_words: 8000, total_words: 10000, percentage: 80.0, contributors: 2, recent_activity: 20 },
  { locale: "ja-JP", translated_words: 3000, total_words: 10000, percentage: 30.0, contributors: 1, recent_activity: 10 },
  { locale: "es-ES", translated_words: 12000, total_words: 15000, percentage: 80.0, contributors: 3, recent_activity: 30 },
  { locale: "pt-BR", translated_words: 6000, total_words: 8000, percentage: 75.0, contributors: 2, recent_activity: 15 },
];

export const mockContributors = [
  { name: "Alice Chen", avatar_url: "", translations: 450, reviews: 120, languages: ["fr-FR", "zh-CN"] },
  { name: "Bob Smith", translations: 320, reviews: 85, languages: ["de-DE"] },
  { name: "Carlos Ruiz", translations: 280, reviews: 60, languages: ["es-ES", "pt-BR"] },
  { name: "Yuki Tanaka", translations: 150, reviews: 40, languages: ["ja-JP"] },
];

export const mockTerms = [
  { id: "t-1", term: "workspace", definition: "A top-level organizational unit containing projects", locale: "en-US", domain: "product" },
  { id: "t-2", term: "stream", definition: "A named branch of content within a project", locale: "en-US", domain: "technical" },
  { id: "t-3", term: "collection", definition: "A group of items within a project", locale: "en-US", translations: { "fr-FR": "collection", "de-DE": "Sammlung" } },
];

export const mockActivityData = [
  { date: "Mar 1", value: 12 },
  { date: "Mar 5", value: 25 },
  { date: "Mar 10", value: 18 },
  { date: "Mar 15", value: 35 },
  { date: "Mar 20", value: 42 },
  { date: "Mar 24", value: 38 },
];

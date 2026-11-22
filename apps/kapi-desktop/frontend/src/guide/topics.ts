// The help topics Kapi Desktop shows in its Help panel. Each topic is a
// section of the Kapi Desktop overview page in the docs, read from the page's
// source when the app is built, so the panel and the website show the same
// text. The topic's last line opens that section on the website.

import { topicsFromDocs, type DocsTopicSpec, type HelpTopic } from "@neokapi/ui-primitives";
import overviewSource from "../../../../../web/docs/kapi/desktop/overview.mdx?raw";
import { GUIDE_IDS } from "./guides";

/** The topic ids, for the places that open one. Each is a heading id on the page. */
export const TOPIC_IDS = {
  home: "your-context",
  overview: "overview",
  checks: "checks",
  reader: "reader",
  decide: "decide",
  search: "search-and-explore",
  voiceTerms: "voice-and-terms",
  projectSettings: "project-settings",
  assistants: "assistants",
  share: "share",
  languages: "languages",
  help: "help",
  commandLine: "command-line",
  sample: "sample",
} as const;

/** The docs site the page is published on. */
export const DOCS_SITE = "https://neokapi.github.io";

/** The page's path on the site. */
export const OVERVIEW_PATH = "/kapi/desktop/overview";

/** Every topic, in the order the Help panel lists them. */
export const TOPIC_SPECS: DocsTopicSpec[] = [
  { id: TOPIC_IDS.home, section: TOPIC_IDS.home, guides: [GUIDE_IDS.project] },
  { id: TOPIC_IDS.overview, section: TOPIC_IDS.overview, guides: [GUIDE_IDS.project] },
  { id: TOPIC_IDS.checks, section: TOPIC_IDS.checks, guides: [GUIDE_IDS.check] },
  { id: TOPIC_IDS.reader, section: TOPIC_IDS.reader, guides: [GUIDE_IDS.check] },
  { id: TOPIC_IDS.decide, section: TOPIC_IDS.decide, guides: [GUIDE_IDS.decide] },
  { id: TOPIC_IDS.search, section: TOPIC_IDS.search },
  { id: TOPIC_IDS.voiceTerms, section: TOPIC_IDS.voiceTerms },
  { id: TOPIC_IDS.projectSettings, section: TOPIC_IDS.projectSettings },
  { id: TOPIC_IDS.assistants, section: TOPIC_IDS.assistants, guides: [GUIDE_IDS.assistant] },
  { id: TOPIC_IDS.share, section: TOPIC_IDS.share },
  { id: TOPIC_IDS.languages, section: TOPIC_IDS.languages, guides: [GUIDE_IDS.languages] },
  { id: TOPIC_IDS.help, section: TOPIC_IDS.help },
  { id: TOPIC_IDS.commandLine, section: TOPIC_IDS.commandLine },
  { id: TOPIC_IDS.sample, section: TOPIC_IDS.sample, guides: [GUIDE_IDS.project] },
];

/**
 * The page in each docs language the build has. The translated pages are
 * build output of the docs site, present only where the docs translation ran.
 */
const TRANSLATED: Record<string, string> = Object.fromEntries(
  Object.entries(
    import.meta.glob<string>(
      "../../../../../web/i18n/*/docusaurus-plugin-content-docs/current/kapi/desktop/overview.mdx",
      { query: "?raw", import: "default", eager: true },
    ),
  ).map(([path, source]) => [path.split("/web/i18n/")[1].split("/")[0], source]),
);

/** The topics on one page; a translated page's links stay on its language's site. */
function read(source: string, locale?: string) {
  const siteUrl = locale ? `${DOCS_SITE}/${locale}` : DOCS_SITE;
  return topicsFromDocs({ path: OVERVIEW_PATH, source }, TOPIC_SPECS, { siteUrl });
}

/**
 * Every help topic in the language asked for. A topic the translated page
 * does not have yet is shown from the English page.
 */
export function buildTopics(locale?: string): HelpTopic[] {
  const english = read(overviewSource).topics;
  const translatedSource = locale ? TRANSLATED[locale] : undefined;
  if (!translatedSource) return english;
  const translated = new Map(read(translatedSource, locale).topics.map((tp) => [tp.id, tp]));
  return english.map((tp) => translated.get(tp.id) ?? tp);
}

/** What the English page is missing or carries that the panel cannot show. */
export function topicProblems(): { missing: string[]; unsupported: string[] } {
  const { missing, unsupported } = read(overviewSource);
  return { missing, unsupported };
}

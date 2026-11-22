// Reads help topics out of a docs page. A topic is one section of the page,
// the one whose heading carries the topic's `{#id}`, so the Help panel and the
// website show the same text and a docs change reaches both.

import type { HelpTopic } from "./types";

/** One docs page the topics are read from. */
export interface DocsPage {
  /** The page's path on the docs site, such as `/kapi/desktop/overview`. */
  path: string;
  /** The page's MDX source. */
  source: string;
}

/** A topic the app offers, named by the section it shows. */
export interface DocsTopicSpec {
  /** The topic id the app opens it by. */
  id: string;
  /** The `{#id}` on the section's heading. */
  section: string;
  /** Guides that walk through what the section describes, by id. */
  guides?: string[];
}

export interface DocsTopicsOptions {
  /** The docs site's origin, prefixed to the page's root-relative links. */
  siteUrl: string;
}

export interface DocsTopicsResult {
  /** The topics found, in the order of the specs. */
  topics: HelpTopic[];
  /** Sections a spec names that the page does not have. */
  missing: string[];
  /** Components in a section that the panel cannot show, as `section: <Name>`. */
  unsupported: string[];
}

/** The href a topic body uses to open another topic in the panel. */
export const TOPIC_HREF_PREFIX = "#topic=";

/**
 * Components left out of the panel on purpose. Screenshots and videos show
 * the screen the reader already has in front of them.
 */
const DROPPED_COMPONENTS = new Set(["ThemedImage", "ThemedVideo"]);

const HEADING = /^(#{2,6})\s+(.*?)\s*(?:\{#([A-Za-z0-9_-]+)\})?\s*$/;

interface Section {
  level: number;
  title: string;
  lines: string[];
}

function sectionsOf(source: string): Map<string, Section> {
  const lines = source.split(/\r?\n/);
  const found = new Map<string, Section>();
  // Each open section collects lines until a heading at its level or above.
  const open: { id: string | undefined; level: number }[] = [];
  let fence: string | null = null;
  for (const line of lines) {
    const fenceMark = line.match(/^\s*(```|~~~)/);
    if (fenceMark) fence = fence === null ? fenceMark[1] : fence === fenceMark[1] ? null : fence;
    const heading = fence === null && !fenceMark ? line.match(HEADING) : null;
    if (heading) {
      const level = heading[1].length;
      while (open.length && open[open.length - 1].level >= level) open.pop();
      for (const o of open) if (o.id) found.get(o.id)!.lines.push(line);
      const id = heading[3];
      if (id) found.set(id, { level, title: heading[2], lines: [] });
      open.push({ id, level });
      continue;
    }
    for (const o of open) if (o.id) found.get(o.id)!.lines.push(line);
  }
  return found;
}

function stripComments(text: string): string {
  return text.replace(/\{\/\*[\s\S]*?\*\/\}/g, "");
}

/** Drops the components the panel leaves out and reports the others. */
function stripComponents(lines: string[], section: string, unsupported: string[]): string[] {
  const out: string[] = [];
  let skipping: string | null = null;
  let fence = false;
  for (const line of lines) {
    if (/^\s*(```|~~~)/.test(line)) fence = !fence;
    if (skipping) {
      if (/\/>\s*$/.test(line) || line.includes(`</${skipping}>`)) skipping = null;
      continue;
    }
    const open = fence ? null : line.match(/^\s*<([A-Z][A-Za-z0-9]*)/);
    if (open) {
      const name = open[1];
      if (!DROPPED_COMPONENTS.has(name)) unsupported.push(`${section}: <${name}>`);
      const closed = /\/>\s*$/.test(line) || line.includes(`</${name}>`);
      if (!closed) skipping = name;
      continue;
    }
    out.push(line);
  }
  return out;
}

function rewriteLinks(
  text: string,
  page: DocsPage,
  sectionToTopic: Map<string, string>,
  siteUrl: string,
): string {
  return text.replace(/\]\(([^)\s]+)\)/g, (whole, href: string) => {
    const [path, anchor] = href.split("#", 2) as [string, string | undefined];
    const samePage = path === "" || path === page.path;
    if (samePage && anchor && sectionToTopic.has(anchor)) {
      return `](${TOPIC_HREF_PREFIX}${sectionToTopic.get(anchor)})`;
    }
    if (path === "") return `](${siteUrl}${page.path}#${anchor ?? ""})`;
    if (path.startsWith("/")) return `](${siteUrl}${href})`;
    return whole;
  });
}

/** The section's first sentence, as plain text, for the topic list. */
function summaryOf(body: string): string {
  const paragraph =
    body
      .split(/\n\s*\n/)
      .map((p) => p.trim())
      .find((p) => p && !/^([-*]\s|[|#>]|\d+\.\s)/.test(p)) ?? "";
  const plain = paragraph
    .replace(/\s+/g, " ")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/(\*\*|__|`|_)/g, "");
  const end = plain.search(/[.:](\s|$)/);
  return end === -1 ? plain : plain.slice(0, end + 1).replace(/:$/, ".");
}

function plainTitle(title: string): string {
  return title.replace(/(\*\*|__|`)/g, "");
}

/**
 * The topics the specs name, read from the page. A spec whose section is not
 * on the page is reported in `missing` and left out.
 */
export function topicsFromDocs(
  page: DocsPage,
  specs: DocsTopicSpec[],
  { siteUrl }: DocsTopicsOptions,
): DocsTopicsResult {
  const sections = sectionsOf(stripComments(page.source));
  const sectionToTopic = new Map(specs.map((s) => [s.section, s.id]));
  const topics: HelpTopic[] = [];
  const missing: string[] = [];
  const unsupported: string[] = [];
  for (const spec of specs) {
    const section = sections.get(spec.section);
    if (!section) {
      missing.push(spec.section);
      continue;
    }
    const lines = stripComponents(section.lines, spec.section, unsupported).map((line) =>
      line.replace(/\s*\{#[A-Za-z0-9_-]+\}\s*$/, ""),
    );
    const body = rewriteLinks(lines.join("\n"), page, sectionToTopic, siteUrl)
      .replace(/\n{3,}/g, "\n\n")
      .trim();
    topics.push({
      id: spec.id,
      title: plainTitle(section.title),
      summary: summaryOf(body),
      body,
      guides: spec.guides,
      moreUrl: `${siteUrl}${page.path}#${spec.section}`,
    });
  }
  return { topics, missing, unsupported };
}

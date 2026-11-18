// Live previews: the story that renders a component, and the dictionary a
// running component needs to show a reviewer's translations in place.
//
// A component's strings record the source file they were extracted from, and
// a Storybook's index records the file each story was written in. The two sit
// side by side by convention (`OfflineLaunch.tsx` beside
// `OfflineLaunch.stories.tsx`), so the file's base name joins them.
import type { ContainerUnit, StoryEntry, StoryIndex } from "./types.ts";

/** The message a host posts into a story to render its own strings. */
export const PREVIEW_TRANSLATIONS_MESSAGE = "neokapi-i18n-translations";
/** The message a story posts once it can accept them. */
export const PREVIEW_READY_MESSAGE = "neokapi-i18n-ready";

/** A path's file name without directories, the `.stories` part or the extension. */
export function componentFileName(path: string): string {
  const file = path.split("/").pop() ?? "";
  return file.replace(/\.stories\.(tsx|ts|jsx|js)$/, "").replace(/\.(tsx|ts|jsx|js)$/, "");
}

/** The stories that render the component in sourceFile, in index order, docs pages left out. */
export function storiesForSource(index: StoryIndex, sourceFile: string): StoryEntry[] {
  const name = componentFileName(sourceFile);
  if (!name) return [];
  return Object.values(index.entries ?? {}).filter(
    (e) => e.type !== "docs" && componentFileName(e.importPath) === name,
  );
}

/** The URL that renders one story with no Storybook chrome around it. */
export function storyURL(storybookURL: string, storyId: string): string {
  const base = storybookURL.replace(/\/+$/, "");
  return `${base}/iframe.html?id=${encodeURIComponent(storyId)}&viewMode=story`;
}

/** "Offline Launch · Default": the component, then which of its states. */
export function storyLabel(entry: StoryEntry): string {
  const component = entry.title.split("/").pop() ?? entry.title;
  return `${component} · ${entry.name}`;
}

/**
 * The dictionary a running component renders a container in: message key to
 * translation, for every unit a runtime addresses. Source reading is the
 * empty dictionary, so the component shows its own literals.
 */
export function previewDictionary(
  units: Pick<ContainerUnit, "messageKey" | "runtimeText">[],
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const u of units) {
    if (u.messageKey && u.runtimeText) out[u.messageKey] = u.runtimeText;
  }
  return out;
}

// Story names that usually draw a state other than the component's content:
// an error, a sign-in, an empty or loading screen. They go last.
const STATE_STORY =
  /error|fail|refus|denied|permission|signed.?out|sign.?in|empty|loading|offline|unavailable/i;

/**
 * The order a reviewer steps through a component's stories in: a story named
 * for the component first, then the rest in index order, with dark variants
 * and stories named for an error or an empty state at the end.
 */
export function rankStories(stories: StoryEntry[], component: string): StoryEntry[] {
  const want = component.toLowerCase();
  const score = (e: StoryEntry): number => {
    const name = `${e.title.split("/").pop() ?? ""} ${e.name}`.toLowerCase();
    let s = 0;
    if (want && e.name.toLowerCase().includes(want)) s -= 10;
    if (/dark/.test(name)) s += 2;
    if (STATE_STORY.test(e.name)) s += 5;
    return s;
  };
  return stories
    .map((e, i) => ({ e, i, s: score(e) }))
    .sort((a, b) => a.s - b.s || a.i - b.i)
    .map((x) => x.e);
}

/**
 * The stretches of plain text a reader sees of a runtime string, longest
 * first: the text between its placeholders, codes and ICU syntax. A running
 * component draws each of them as written, so they find the string on the
 * page.
 */
export function visibleFragments(runtime: string, max = 3): string[] {
  // Innermost braces go first, so an ICU plural or select goes whole: its
  // forms are picked at render time and match nothing as written.
  let flat = runtime;
  for (let prev = ""; prev !== flat;) {
    prev = flat;
    flat = flat.replace(/\{[^{}]*\}/g, "\uE000");
  }
  const parts = flat
    .split(/\uE000|\[[^\]]*\]|[{}[\]]|<\/?[^<>]*>/)
    .map((p) => p.replace(/\s+/g, " ").trim())
    .filter((p) => /\p{L}.*\p{L}/u.test(p));
  return [...new Set(parts)].sort((a, b) => b.length - a.length).slice(0, max);
}

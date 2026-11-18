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

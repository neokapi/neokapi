import { useSyncExternalStore } from "react";

/** The colour mode the app has chosen: the `dark` class on the root element. */
export type RootColorMode = "light" | "dark";

function read(): RootColorMode {
  if (typeof document === "undefined") return "light";
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

function subscribe(onChange: () => void): () => void {
  if (typeof document === "undefined") return () => {};
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

/**
 * The app's theme, which both desktop apps and the web app mark with the
 * `dark` class on the root element and no class for light. The flow editor
 * hands it to React Flow as `forceColorMode`; without it React Flow follows
 * the system preference, and a light app on a dark-mode machine drew a dark
 * canvas.
 */
export function useRootColorMode(): RootColorMode {
  return useSyncExternalStore(subscribe, read, () => "light");
}

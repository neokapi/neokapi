// Color-mode plumbing for the landing. Light is the default; `.dark` on
// <html> switches. Persisted in localStorage, override with ?mode=light|dark
// (used by the screenshot tooling).

const MODE_KEY = "bowrain-mode";

export function initTheme(): void {
  const params = new URLSearchParams(window.location.search);
  const modeParam = params.get("mode");
  const stored = localStorage.getItem(MODE_KEY);
  const mode = modeParam === "dark" || modeParam === "light" ? modeParam : (stored ?? "light");
  document.documentElement.classList.toggle("dark", mode === "dark");
}

export function toggleMode(): boolean {
  const dark = document.documentElement.classList.toggle("dark");
  localStorage.setItem(MODE_KEY, dark ? "dark" : "light");
  return dark;
}

export function isDark(): boolean {
  return document.documentElement.classList.contains("dark");
}

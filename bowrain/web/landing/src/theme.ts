// Theme + brand-candidate plumbing for the landing.
//
// Mode: light is the default; `.dark` on <html> switches. Persisted in
// localStorage, override with ?mode=light|dark.
// Brand: the identity-candidate palette, set as [data-brand] on <html>;
// override with ?brand=rainlight|indigo|graphite|petrichor (used by the
// screenshot pass during identity selection).

const BRANDS = ["rainlight", "indigo", "graphite", "petrichor"] as const;
export type Brand = (typeof BRANDS)[number];

const MODE_KEY = "bowrain-mode";

export function initTheme(): void {
  const params = new URLSearchParams(window.location.search);

  const brandParam = params.get("brand");
  if (brandParam && (BRANDS as readonly string[]).includes(brandParam)) {
    document.documentElement.dataset.brand = brandParam;
  }

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

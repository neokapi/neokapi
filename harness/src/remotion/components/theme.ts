import { loadFont as loadInter } from "@remotion/google-fonts/Inter";
import { loadFont as loadMono } from "@remotion/google-fonts/JetBrainsMono";

// Load only the weights/subsets we use. latin-ext covers French accents (é è à ç …)
// that show up in translated terminal output and captions.
export const inter = loadInter("normal", {
  weights: ["400", "500", "600", "700"],
  subsets: ["latin", "latin-ext"],
});
export const mono = loadMono("normal", {
  weights: ["400", "600"],
  subsets: ["latin", "latin-ext"],
});

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;

export type ThemeMode = "light" | "dark";

// Dark palette (default). The light palette below mirrors every key so the same
// composition can render for the docs' light mode — a theme-matched video instead
// of a dark video blending into a dark page.
export const darkTheme = {
  bg: "#0b0e17",
  bgGrad: "radial-gradient(1400px 900px at 75% -15%, #18223e 0%, #0d1222 55%, #080b13 100%)",
  panel: "#0f1422",
  panelBorder: "rgba(255,255,255,0.09)",
  chrome: "#161c2c",
  text: "#e9edf7",
  dim: "#8b97b6",
  faint: "#5c6788",
  accent: "#7aa2ff",
  accent2: "#b78bff",
  green: "#7fe0a0",
  amber: "#ffd479",
  red: "#ff97a6",
  user: "#9ad0ff",
  toolBg: "rgba(122,162,255,0.08)",
  toolBorder: "rgba(122,162,255,0.28)",
  resultBg: "rgba(255,255,255,0.035)",
  fontSans: inter.fontFamily,
  fontMono: mono.fontFamily,

  // ── Claude Code terminal palette (authentic CLI feel) ──
  termBg: "#1a1b20", // terminal background (neutral dark)
  termText: "#e8e6e3", // off-white foreground
  termDim: "#8b8985", // dimmed (tool results, hints)
  termFaint: "#5f5e5a",
  termGreen: "#4eb87b", // shell $ commands / success
  termRed: "#e0707f", // errors
};

export type Theme = typeof darkTheme;

// Light palette — same keys, tuned for a white docs page. Backgrounds go light,
// foregrounds go dark; accents shift to AA-contrast variants. The captured app /
// document artifacts are real screenshots (already bright) and are unaffected.
export const lightTheme: Theme = {
  bg: "#f5f7fb",
  bgGrad: "radial-gradient(1400px 900px at 75% -15%, #eaf0fb 0%, #f3f6fc 55%, #ffffff 100%)",
  panel: "#ffffff",
  panelBorder: "rgba(15,23,42,0.12)",
  chrome: "#eef1f7",
  text: "#141a29",
  dim: "#566179",
  faint: "#94a0b8",
  accent: "#4f46e5",
  accent2: "#7c3aed",
  green: "#1f7a44",
  amber: "#8a6310",
  red: "#b3261e",
  user: "#2563a8",
  toolBg: "rgba(79,70,229,0.07)",
  toolBorder: "rgba(79,70,229,0.22)",
  resultBg: "rgba(15,23,42,0.04)",
  fontSans: inter.fontFamily,
  fontMono: mono.fontFamily,

  // Claude Code LIGHT terminal palette
  termBg: "#faf9f7",
  termText: "#2a2826",
  termDim: "#6f6d68",
  termFaint: "#a3a19b",
  termGreen: "#1f7a44",
  termRed: "#c0392b",
};

// The active palette is swapped once per render (see Demo, which calls setTheme
// from the themeMode prop). Every `theme.X` read resolves through this proxy, so
// no call site needs to know which mode is active. Within a single render job the
// mode is constant, so there is no cross-frame flicker.
let activeTheme: Theme = darkTheme;
export function setTheme(mode: ThemeMode): void {
  activeTheme = mode === "light" ? lightTheme : darkTheme;
}
export const theme = new Proxy({} as Theme, {
  get: (_t, key: string) => (activeTheme as Record<string, string>)[key],
}) as Theme;

/** kapi brand orange-ish used sparingly for accents/marks. */
export const KAPI = "#ff7a45";
/** Claude (Anthropic) coral — the ✻ logo + accent in the Claude Code CLI. */
export const CLAUDE = "#d97757";

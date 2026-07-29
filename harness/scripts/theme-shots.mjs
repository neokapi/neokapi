// Theme-candidate screenshot pass for the identity-selection round.
// Plants the seeded session cookie, then captures the real web app under each
// candidate palette (packages/ui/src/styles/candidates/*.css) in light+dark.
// Run right after `make harness-seed` (tokens live ~15 min):
//   node scripts/theme-shots.mjs <outDir>
import { chromium } from "playwright";
import { readFileSync, mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HARNESS = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const REPO = path.dirname(HARNESS);
const OUT = process.argv[2] ?? path.join(HARNESS, "theme-shots");
mkdirSync(OUT, { recursive: true });

const env = Object.fromEntries(
  readFileSync(path.join(HARNESS, ".env"), "utf8")
    .split("\n")
    .filter((l) => l.includes("="))
    .map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]),
);
const BASE = env.BOWRAIN_BACKEND_URL ?? "http://localhost:8080";
const API = `${BASE}/api/v1`;

// Mint a fresh 15-min JWT via the server's device flow (same as the seeder),
// so each context starts with a full TTL regardless of how long a pass takes.
async function deviceAuth(email, name) {
  const form = (url, body) =>
    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
      redirect: "manual",
    });
  const start = await (await form(`${API}/auth/device/start`, "client_id=e2e-shared")).json();
  await form(
    `${API}/auth/device/verify`,
    `user_code=${start.user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
  );
  const poll = await (
    await form(
      `${API}/auth/device/poll`,
      `device_code=${start.device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
    )
  ).json();
  if (!poll.access_token) throw new Error(`device auth (${email}): no access_token`);
  return poll.access_token;
}

const BRANDS = ["rainlight", "indigo", "graphite", "petrichor"];
const WS = env.BOWRAIN_WORKSPACE_SLUG ?? "bowmart";
const PID = env.BOWRAIN_PROJECT_ID ?? "utulp9hb";
const ITEM = env.BOWRAIN_ITEM_ID ?? "4neqRudD";
const ROUTES = [
  { id: "dashboard", path: `/${WS}` },
  { id: "editor", path: `/${WS}/p/${PID}/s/main/${ITEM}/translate` },
  { id: "voice", path: `/${WS}/context/voice` },
];

const css = {};
for (const b of BRANDS) {
  css[b] = readFileSync(
    path.join(REPO, "packages/ui/src/styles/candidates", `${b}.css`),
    "utf8",
  );
}

const browser = await chromium.launch();
const u = new URL(BASE);

for (const mode of ["light", "dark"]) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
    colorScheme: mode,
  });
  const token = await deviceAuth("admin@example.com", "Alex Rivera");
  await context.addCookies([
    {
      name: "bowrain_session",
      value: token,
      domain: u.hostname,
      path: "/api/",
      httpOnly: true,
      sameSite: "Lax",
      secure: false,
    },
    // The app's theme preference cookie (theme provider reads it on boot).
    { name: "neokapi-theme", value: mode, domain: u.hostname, path: "/", sameSite: "Lax", secure: false },
  ]);
  const page = await context.newPage();
  await page.addInitScript((m) => {
    try {
      localStorage.setItem("neokapi-theme", m);
      localStorage.setItem("theme", m);
    } catch {}
  }, mode);

  for (const route of ROUTES) {
    await page.goto(`${BASE}${route.path}`, { waitUntil: "domcontentloaded" }).catch(() => {});
    // settle data fetches/fonts (networkidle never fires — the app holds SSE open)
    await page.waitForTimeout(3500);
    for (const brand of BRANDS) {
      await page.evaluate(
        ({ text, m }) => {
          document.getElementById("brand-override")?.remove();
          const s = document.createElement("style");
          s.id = "brand-override";
          s.textContent = text;
          document.head.appendChild(s);
          document.documentElement.classList.toggle("dark", m === "dark");
        },
        { text: css[brand], m: mode },
      );
      await page.waitForTimeout(250);
      const file = path.join(OUT, `app-${route.id}-${brand}-${mode}.png`);
      await page.screenshot({ path: file });
      console.log("✓", file);
    }
  }
  await context.close();
}
await browser.close();
console.log("done");

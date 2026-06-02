#!/usr/bin/env -S vpx tsx
/**
 * Unified, idempotent bowrain seeder for the staged video pipeline.
 *
 * Provisions ONE shared workspace (`bowmart`) holding everything all five
 * bowrain-web walkthroughs need, then writes the record-phase tokens + ids to
 * `harness/.env` (which the recorder's loadEnv() reads). This supersedes the
 * per-demo seed-collaboration.mjs / seed-correction-loop.mjs scripts for the
 * staged pass: those each minted a separate, uniquely-slugged workspace and a
 * different token, but the recorder resolves ONE workspace via
 * bowrainWorkspaceSlug() and reads ONE BOWRAIN_SESSION_TOKEN.
 *
 * Auth uses the bowrain server's own device flow (no Keycloak password). All
 * creation is check-then-create against a FIXED slug, so re-running reuses the
 * workspace/projects/profile and just re-mints tokens + rewrites .env — no
 * accretion, no duplicate projects, no exploding correction counts.
 *
 *   make -C bowrain stack-up-web        # stack must be running
 *   vpx tsx scripts/seed-bowrain.ts     # (or: make -C harness seed)
 *
 * Tokens expire in 15 min, so the staged target runs this immediately before
 * the record phase.
 */
import fs from "node:fs";
import path from "node:path";

const BASE = process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";
const API = `${BASE}/api/v1`;
const SLUG = "bowmart";

// Two real device-flow users: Alice owns the workspace + is the on-camera user;
// Bob is the off-camera teammate whose live presence the collaboration walk
// records. (Content inside the project is still the proven demo content; a
// BowMart content rebrand is a follow-up that moves with the walk anchors.)
const ALICE = {
  email: process.env.BOWRAIN_ALICE_EMAIL || "admin@example.com",
  name: process.env.BOWRAIN_ALICE_NAME || "Alex Rivera",
};
const BOB = {
  email: process.env.BOWRAIN_BOB_EMAIL || "maria@acme.example",
  name: process.env.BOWRAIN_BOB_NAME || "Maria Schmidt",
};

const FILE_NAME = "about-us.html";
const COLLAB_LOCALE = "fr";

// ── low-level HTTP ──────────────────────────────────────────────────────────

interface DeviceStart {
  device_code: string;
  user_code: string;
}
interface DevicePoll {
  access_token?: string;
}

/** Run the bowrain device flow for an email/name and return a bowrain JWT. */
async function deviceAuth(email: string, name: string): Promise<string> {
  const form = (body: string) =>
    fetch(`${API}/auth/device/start`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });
  const start = (await (await form("client_id=e2e-shared")).json()) as DeviceStart;
  await fetch(`${API}/auth/device/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `user_code=${start.user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
    redirect: "manual",
  });
  const poll = (await (
    await fetch(`${API}/auth/device/poll`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: `device_code=${start.device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
    })
  ).json()) as DevicePoll;
  if (!poll.access_token) throw new Error(`device auth (${email}): no access_token`);
  return poll.access_token;
}

const authJSON = (token: string) => ({
  Authorization: `Bearer ${token}`,
  "Content-Type": "application/json",
});

async function jget<T>(p: string, token: string): Promise<T> {
  const r = await fetch(`${API}${p}`, { headers: { Authorization: `Bearer ${token}` } });
  if (!r.ok) throw new Error(`GET ${p} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
  return (await r.json()) as T;
}

async function jpost<T>(p: string, body: unknown, token: string): Promise<T> {
  const r = await fetch(`${API}${p}`, {
    method: "POST",
    headers: authJSON(token),
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`POST ${p} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
  return (await r.json()) as T;
}

/** Best-effort POST — used for TM/terms whose duplicates are visually harmless. */
async function jpostSoft(p: string, body: unknown, token: string): Promise<void> {
  try {
    await jpost(p, body, token);
  } catch (e) {
    console.error(`  (skipped ${p}: ${(e as Error).message})`);
  }
}

function listOf<T>(data: unknown, key: string): T[] {
  if (Array.isArray(data)) return data as T[];
  const obj = (data ?? {}) as Record<string, unknown>;
  return (Array.isArray(obj[key]) ? (obj[key] as T[]) : []) as T[];
}

async function uploadIfAbsent(
  ws: string,
  pid: string,
  token: string,
  fileName: string,
  content: string,
): Promise<void> {
  const proj = await jget<{ items?: Array<{ name: string }> }>(`/${ws}/${pid}`, token);
  if ((proj.items ?? []).some((i) => i.name === fileName)) {
    console.log(`  · ${fileName} already uploaded`);
    return;
  }
  const form = new FormData();
  form.append("files", new Blob([content], { type: "text/html" }), fileName);
  const r = await fetch(`${API}/${ws}/${pid}/items/main`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  if (!r.ok) throw new Error(`upload ${fileName} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
  console.log(`  · uploaded ${fileName}`);
}

// ── idempotent provisioning ─────────────────────────────────────────────────

interface Workspace {
  slug: string;
}
interface Project {
  id?: string;
  name: string;
  project?: { id?: string };
}
interface BrandProfile {
  id: string;
  name: string;
}
interface Member {
  email?: string;
  user?: { email?: string };
}

async function ensureWorkspace(token: string): Promise<string> {
  const existing = listOf<Workspace>(await jget("/workspaces", token), "workspaces");
  if (existing.some((w) => w.slug === SLUG)) {
    console.log(`  · workspace ${SLUG} exists`);
    return SLUG;
  }
  const ws = await jpost<Workspace>(
    "/workspaces",
    { name: "BowMart Localization", slug: SLUG },
    token,
  );
  console.log(`  · created workspace ${SLUG}`);
  return ws.slug || SLUG;
}

async function ensureProject(
  ws: string,
  token: string,
  name: string,
  src: string,
  targets: string[],
): Promise<string> {
  const existing = listOf<Project>(await jget(`/${ws}/projects`, token), "projects");
  const match = existing.find((p) => p.name === name);
  if (match) {
    console.log(`  · project "${name}" exists`);
    return (match.id || match.project?.id) as string;
  }
  const p = await jpost<Project>(
    `/${ws}/projects`,
    { name, default_source_language: src, target_languages: targets },
    token,
  );
  console.log(`  · created project "${name}"`);
  return (p.id || p.project?.id) as string;
}

async function ensureBrandProfile(ws: string, token: string, name: string): Promise<string> {
  const existing = listOf<BrandProfile>(
    await jget(`/${ws}/brand-profiles`, token),
    "brand_profiles",
  );
  const match = existing.find((p) => p.name === name);
  if (match) {
    console.log(`  · brand profile "${name}" exists`);
    return match.id;
  }
  const p = await jpost<BrandProfile>(
    `/${ws}/brand-profiles`,
    {
      name,
      description: "Acme's brand voice — clear, direct, no jargon.",
      tone: {
        personality: ["clear", "direct"],
        formality: "neutral",
        emotion: "warm",
        humor: "light",
      },
    },
    token,
  );
  console.log(`  · created brand profile "${name}"`);
  return p.id;
}

async function ensureMember(ws: string, aliceToken: string, bobToken: string): Promise<boolean> {
  const members = listOf<Member>(await jget(`/${ws}/members`, aliceToken), "members");
  if (members.some((m) => (m.email || m.user?.email) === BOB.email)) {
    console.log(`  · ${BOB.email} already a member`);
    return true;
  }
  const invite = await jpost<{ code?: string; invite?: { code?: string } }>(
    `/${ws}/invites`,
    { role: "member", email: BOB.email, max_uses: 1, ttl_days: 1 },
    aliceToken,
  );
  const code = invite.code || invite.invite?.code;
  if (!code) throw new Error(`no invite code: ${JSON.stringify(invite)}`);
  await jpost(`/join/${code}`, {}, bobToken);
  const bobWs = listOf<Workspace>(await jget("/workspaces", bobToken), "workspaces");
  const joined = bobWs.some((w) => w.slug === ws);
  console.log(`  · invited + joined ${BOB.email} (joined=${joined})`);
  return joined;
}

// ── demo content (proven; same as the two reference seeds) ───────────────────

const ABOUT_US_HTML = `<!doctype html>
<html lang="en">
  <head><meta charset="UTF-8" /><title>About Us - Acme Inc.</title></head>
  <body>
    <header>
      <h1>About Acme Inc.</h1>
      <p>Building the future of <strong>cloud infrastructure</strong> since 2018.</p>
    </header>
    <section id="mission">
      <h2>Our Mission</h2>
      <p>We believe every developer deserves reliable, fast, and secure infrastructure. Our platform
        handles over <em>10 million</em> requests per day across 42 countries.</p>
      <p>From startups to Fortune 500 companies, our customers trust us with their most critical
        workloads. We take that responsibility seriously.</p>
    </section>
    <section id="team">
      <h2>Our Team</h2>
      <p>We are a distributed team of 120 engineers, designers, and product specialists across
        <a href="/offices">12 offices worldwide</a>.</p>
    </section>
    <section id="values">
      <h2>Our Values</h2>
      <ul>
        <li><strong>Transparency</strong> &mdash; We share our roadmap and pricing openly.</li>
        <li><strong>Reliability</strong> &mdash; We maintain 99.99% uptime across all services.</li>
        <li><strong>Security</strong> &mdash; SOC 2 Type II certified with end-to-end encryption.</li>
      </ul>
    </section>
    <section id="contact">
      <h2>Get in Touch</h2>
      <p>Have questions? Reach out at <a href="mailto:hello@acme-inc.example">hello@acme-inc.example</a>.</p>
    </section>
  </body>
</html>
`;

const MARKETING_HTML = `<!doctype html>
<html lang="en">
  <head><meta charset="UTF-8" /><title>Acme — Marketing</title></head>
  <body>
    <h1>Utilize Acme to ship faster</h1>
    <p>Teams utilize our platform to leverage their existing infrastructure and
      utilize every hour of the day. We help you leverage automation.</p>
    <section>
      <h2>Best-in-class synergy</h2>
      <p>Our best-in-class tooling drives synergy across your org. Leverage the
        synergy of a best-in-class platform and utilize proven workflows.</p>
    </section>
    <section>
      <h2>Why teams leverage Acme</h2>
      <p>Utilize one dashboard. Leverage one pipeline. Best-in-class support,
        real synergy, and a platform teams utilize daily.</p>
    </section>
  </body>
</html>
`;

const TM_ENTRIES = [
  {
    source: "About Acme Inc.",
    target: "À propos d'Acme Inc.",
    source_locale: "en",
    target_locale: "fr",
  },
  { source: "Our Mission", target: "Notre mission", source_locale: "en", target_locale: "fr" },
  { source: "Our Team", target: "Notre équipe", source_locale: "en", target_locale: "fr" },
  { source: "Our Values", target: "Nos valeurs", source_locale: "en", target_locale: "fr" },
  { source: "Get in Touch", target: "Contactez-nous", source_locale: "en", target_locale: "fr" },
  {
    source: "We believe every developer deserves reliable, fast, and secure infrastructure.",
    target:
      "Nous pensons que chaque développeur mérite une infrastructure fiable, rapide et sécurisée.",
    source_locale: "en",
    target_locale: "fr",
  },
  { source: "Our Mission", target: "Unsere Mission", source_locale: "en", target_locale: "de" },
  { source: "Our Team", target: "Unser Team", source_locale: "en", target_locale: "de" },
];

const CONCEPTS = [
  {
    domain: "cloud",
    definition: "Managed, multi-tenant compute and storage delivered over the network.",
    terms: [
      { text: "cloud infrastructure", locale: "en", status: "preferred" },
      { text: "infrastructure cloud", locale: "fr", status: "preferred" },
      { text: "Cloud-Infrastruktur", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "reliability",
    definition: "The proportion of time a service is operational and reachable.",
    terms: [
      { text: "uptime", locale: "en", status: "preferred" },
      { text: "disponibilité", locale: "fr", status: "preferred" },
      { text: "Verfügbarkeit", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "security",
    definition: "Protecting data so only authorised parties can read it, end to end.",
    terms: [
      { text: "encryption", locale: "en", status: "preferred" },
      { text: "chiffrement", locale: "fr", status: "preferred" },
      { text: "cryptage", locale: "fr", status: "deprecated" },
      { text: "Verschlüsselung", locale: "de", status: "preferred" },
    ],
  },
];

// Each (original → corrected) repeated past the min-count threshold (3) so it
// surfaces as a candidate rule on the correction-loop review page.
const CORRECTION_STREAM = [
  { original: "utilize", corrected: "use", n: 5 },
  { original: "leverage", corrected: "use", n: 4 },
  { original: "synergy", corrected: "collaboration", n: 3 },
  { original: "best-in-class", corrected: "proven", n: 3 },
  { original: "kindly", corrected: "please", n: 1 }, // below threshold — stays a non-candidate
];

// ── env bridge ──────────────────────────────────────────────────────────────

function writeEnv(vars: Record<string, string>): string {
  const envPath = path.resolve(import.meta.dirname!, "..", ".env");
  const body = Object.entries(vars)
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
  fs.writeFileSync(envPath, `${body}\n`);
  return envPath;
}

// ── orchestration ────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  console.log(`Seeding bowrain @ ${BASE} (workspace=${SLUG}) …`);

  // Wait for the server to be ready (the staged target may call us right after
  // `docker compose up`, and the one-shot web-init makes --wait unreliable).
  await waitForServer();

  const aliceToken = await deviceAuth(ALICE.email, ALICE.name);
  const bobToken = await deviceAuth(BOB.email, BOB.name);

  const ws = await ensureWorkspace(aliceToken);

  // Project 1 — "Company Website": the editor / review / collaboration walks.
  const projectId = await ensureProject(ws, aliceToken, "Company Website", "en", [
    "fr",
    "de",
    "ja",
  ]);
  await uploadIfAbsent(ws, projectId, aliceToken, FILE_NAME, ABOUT_US_HTML);
  const proj = await jget<{ items?: Array<{ id: string; name: string }> }>(
    `/${ws}/${projectId}`,
    aliceToken,
  );
  const item = (proj.items ?? []).find((i) => i.name === FILE_NAME) ?? (proj.items ?? [])[0];
  const itemId = item?.id;
  if (!itemId)
    throw new Error(`no item id for ${FILE_NAME} (items: ${JSON.stringify(proj.items)})`);

  // Pre-translate fr so the review walk has translated-but-unreviewed rows and
  // the editor shows target content. Best-effort (offline demo provider).
  await jpostSoft(
    `/${ws}/${projectId}/actions/main/ai-translate`,
    { item: FILE_NAME, target_locale: COLLAB_LOCALE },
    aliceToken,
  );

  // TM + terminology: the governance walk (TM search "mission", multi-locale
  // concepts) + the editor context panel. Best-effort, sequential.
  for (const e of TM_ENTRIES)
    await jpostSoft(`/${ws}/translation-memory`, { ...e, project_id: projectId }, aliceToken);
  for (const c of CONCEPTS)
    await jpostSoft(`/${ws}/terms`, { ...c, project_id: projectId }, aliceToken);

  // Bob joins (collaboration walk).
  const joined = await ensureMember(ws, aliceToken, bobToken);

  // Brand profile + Project 2 "Marketing Site" (the correction-loop dropdown
  // needs a SECOND project) + off-brand content + correction stream.
  const profileId = await ensureBrandProfile(ws, aliceToken, "Acme Voice");
  const marketingId = await ensureProject(ws, aliceToken, "Marketing Site", "en", ["fr", "de"]);
  await uploadIfAbsent(ws, marketingId, aliceToken, "marketing.html", MARKETING_HTML);

  // Guard: only post the correction stream if candidates aren't already there,
  // else re-running multiplies the counts and the demo shows wrong numbers.
  const existingCands = listOf<unknown>(
    await jget(`/${ws}/brand-profiles/${profileId}/candidates?min_count=3`, aliceToken),
    "candidates",
  );
  if (existingCands.length > 0) {
    console.log(
      `  · correction candidates already present (${existingCands.length}) — skipping stream`,
    );
  } else {
    let posted = 0;
    for (const c of CORRECTION_STREAM) {
      for (let i = 0; i < c.n; i++) {
        await jpostSoft(
          `/${ws}/${marketingId}/brand-voice/main/corrections`,
          {
            profile_id: profileId,
            block_id: `seed-${c.original}-${i}`,
            dimension: "vocabulary",
            original_text: c.original,
            corrected_text: c.corrected,
          },
          aliceToken,
        );
        posted++;
      }
    }
    console.log(`  · posted ${posted} corrections`);
  }

  const envPath = writeEnv({
    BOWRAIN_BACKEND_URL: BASE,
    BOWRAIN_SESSION_TOKEN: aliceToken,
    BOWRAIN_PEER_TOKEN: bobToken,
    BOWRAIN_PEER_NAME: BOB.name,
    BOWRAIN_WORKSPACE_SLUG: ws,
    BOWRAIN_PROJECT_ID: projectId,
    BOWRAIN_ITEM_ID: itemId,
    BOWRAIN_COLLAB_LOCALE: COLLAB_LOCALE,
    BOWRAIN_DEMO_PROFILE_ID: profileId,
  });

  console.log("\n✓ seed complete");
  console.log(`  workspace : ${BASE}/${ws}`);
  console.log(`  project   : Company Website (${projectId}), item ${itemId}`);
  console.log(`  marketing : Marketing Site (${marketingId})`);
  console.log(`  brand     : Acme Voice (${profileId})`);
  console.log(`  peer Bob  : joined=${joined}`);
  console.log(`  wrote     : ${envPath}`);
}

/** Poll the server until it answers (handles stack-up race + spurious --wait). */
async function waitForServer(timeoutMs = 60_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const r = await fetch(`${BASE}/`, { signal: AbortSignal.timeout(3000) });
      if (r.ok || r.status === 401) return;
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) throw new Error(`server at ${BASE} not ready after ${timeoutMs}ms`);
    await new Promise((res) => setTimeout(res, 2000));
  }
}

main().catch((e) => {
  console.error("seed failed:", (e as Error).message);
  process.exit(1);
});

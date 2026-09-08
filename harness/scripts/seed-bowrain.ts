#!/usr/bin/env -S vpx tsx
/**
 * Unified, idempotent bowrain seeder for the staged video pipeline.
 *
 * Provisions ONE shared workspace (`bowmart`) holding everything all five
 * bowrain-web walkthroughs need, including the review walk's separation-of-
 * duties state, then writes the record-phase tokens + ids to `harness/.env`
 * (which the recorder's loadEnv() reads). This supersedes the
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
import {
  describeReadiness,
  isRunnable,
  needsPendingWork,
  needsSettle,
  targetsToClear,
} from "../src/lib/seed-state.ts";
import type { ConvergenceEstimate, EditorBlock } from "../src/lib/seed-state.ts";

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

// The locale the automations walk translates on camera. The seed pre-translates
// the other two targets, so this one is the project's pending work: "Translate
// all now" is offered, a real run starts, and its row lands in the runs table.
const PENDING_LOCALE = "ja";

// The two blocks the review walk addresses, named by their source text: the
// block list comes back in id order and ids are minted per seed, so a position
// in it names a different heading every time. Bob's block carries a French
// rendering of its own heading, because the editor and collaboration walks
// open it beside the source and the memory learns the pair.
const PEER_BLOCK_SOURCE = "About Acme Inc.";
const PEER_BLOCK_TARGET = "À propos de la société Acme Inc.";
const SELF_BLOCK_SOURCE = "Our Mission";

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

/**
 * Best-effort POST for content memory and terms: a refused write is logged and
 * the seed goes on, so one rejected entry does not stop the pass. A 404 is
 * different: it means the script posts to a route the server no longer serves,
 * and it fails the seed rather than leaving a card empty in the recording.
 */
async function jpostSoft(p: string, body: unknown, token: string): Promise<void> {
  try {
    await jpost(p, body, token);
  } catch (e) {
    const message = (e as Error).message;
    if (/ → 404:/.test(message)) throw new Error(`route not served: ${message}`);
    console.error(`  (skipped ${p}: ${message})`);
  }
}

async function jput<T>(p: string, body: unknown, token: string): Promise<T> {
  const r = await fetch(`${API}${p}`, {
    method: "PUT",
    headers: authJSON(token),
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`PUT ${p} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
  const text = await r.text();
  return (text ? JSON.parse(text) : {}) as T;
}

async function jdelete(p: string, token: string): Promise<void> {
  const r = await fetch(`${API}${p}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok && r.status !== 404) {
    throw new Error(`DELETE ${p} → ${r.status}: ${(await r.text()).slice(0, 200)}`);
  }
}

/**
 * The collaboration + correction-loop seeders mint timestamp-suffixed workspaces
 * (collab-<n>, brand-loop-<n>) every run and never remove the old ones, so the
 * demo user's `kapi workspace list` accretes test artifacts. Prune those strays
 * here so the workspace list stays the canonical set (the bowmart workspace, plus
 * whatever a fresh collab/brand-loop seed creates for those demos).
 */
async function pruneStrayWorkspaces(token: string): Promise<void> {
  const all = listOf<Workspace>(await jget("/workspaces", token), "workspaces");
  const stray = all.filter((w) => /^(collab|brand-loop)-\d+$/.test(w.slug));
  for (const w of stray) {
    await jdelete(`/${w.slug}`, token);
    console.log(`  · pruned stray workspace ${w.slug}`);
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
  user_id?: string;
  user?: { id?: string };
}
interface Invite {
  id: string;
  email?: string;
  use_count?: number;
}

async function ensureWorkspace(token: string): Promise<string> {
  const existing = listOf<Workspace>(await jget("/workspaces", token), "workspaces");
  if (existing.some((w) => w.slug === SLUG)) {
    console.log(`  · workspace ${SLUG} exists`);
    return SLUG;
  }
  const ws = await jpost<Workspace>(
    "/workspaces",
    { name: "BowMart", slug: SLUG },
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
    await jget(`/${ws}/voice-profiles`, token),
    "brand_profiles",
  );
  const match = existing.find((p) => p.name === name);
  if (match) {
    console.log(`  · voice profile "${name}" exists`);
    return match.id;
  }
  const p = await jpost<BrandProfile>(
    `/${ws}/voice-profiles`,
    {
      name,
      description: "Acme's voice profile — clear, direct, no jargon.",
      tone: {
        personality: ["clear", "direct"],
        formality: "neutral",
        emotion: "warm",
        humor: "light",
      },
    },
    token,
  );
  console.log(`  · created voice profile "${name}"`);
  return p.id;
}

/**
 * Bob joins the workspace through a real invitation, once. The member list
 * carries user ids and no emails, so membership is read against Bob's own id;
 * matching on an email that is never there re-invited him on every run and the
 * unused invitations piled up on the Members page the collaboration walk films.
 * Invitations for Bob that were never used are removed on the way through.
 */
async function ensureMember(ws: string, aliceToken: string, bobToken: string): Promise<boolean> {
  const bobMe = await jget<{ id?: string; user?: { id?: string } }>("/auth/me", bobToken);
  const bobId = bobMe.id || bobMe.user?.id;
  const members = listOf<Member>(await jget(`/${ws}/members`, aliceToken), "members");
  const isMember = !!bobId && members.some((m) => (m.user_id || m.user?.id) === bobId);
  const invites = listOf<Invite>(await jget(`/${ws}/invites`, aliceToken), "invites");
  for (const inv of invites) {
    if (inv.email !== BOB.email || (inv.use_count ?? 0) > 0) continue;
    if (!isMember) continue;
    await jdelete(`/${ws}/invites/${inv.id}`, aliceToken);
    console.log(`  · removed an unused invitation for ${BOB.email}`);
  }
  if (isMember) {
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

interface MemoryEntry {
  source?: string;
  target?: string;
  source_language?: string;
  target_language?: string;
}
interface Concept {
  domain?: string;
  definition?: string;
}

/** Post each content-memory entry the walks read, skipping the ones already
 *  in the workspace store (matched on source text and language pair). */
async function ensureMemoryEntries(ws: string, projectId: string, token: string): Promise<void> {
  const have = listOf<MemoryEntry>(await jget(`/${ws}/translation-memory`, token), "entries");
  const key = (e: MemoryEntry) => `${e.source_language}→${e.target_language}: ${e.source}`;
  const present = new Set(have.map(key));
  let posted = 0;
  for (const e of MEMORY_ENTRIES) {
    if (present.has(key({ source: e.source, source_language: e.source_locale, target_language: e.target_locale })))
      continue;
    await jpostSoft(`/${ws}/translation-memory`, { ...e, project_id: projectId }, token);
    posted++;
  }
  console.log(`  · content memory: ${posted} entries posted, ${MEMORY_ENTRIES.length - posted} already present`);
}

/** Create each concept the walks read, skipping the ones the workspace already
 *  holds (matched on domain and definition). */
async function ensureConcepts(ws: string, projectId: string, token: string): Promise<void> {
  const have = listOf<Concept>(await jget(`/${ws}/concepts`, token), "concepts");
  const present = new Set(have.map((c) => `${c.domain}: ${c.definition}`));
  let posted = 0;
  for (const c of CONCEPTS) {
    if (present.has(`${c.domain}: ${c.definition}`)) continue;
    await jpostSoft(`/${ws}/concepts`, { ...c, project_id: projectId }, token);
    posted++;
  }
  console.log(`  · concepts: ${posted} created, ${CONCEPTS.length - posted} already present`);
}

interface Role {
  id: string;
  name?: string;
  permission_names?: string[];
}
interface ProjectMember {
  user_id?: string;
  user?: { id?: string };
}
interface Block {
  id: string;
  translatable?: boolean;
  source?: string;
}

/**
 * The state the review walk's closing beats read (record-desktop.ts
 * bowrainReviewWalk): a row the recorded user may approve, a row the server
 * refuses her, and a reviewer who can decide that one instead.
 *
 * The workspace policy on approving one's own writing is `warn` by default
 * (auth/governance.go GetSoDMode), which files an audit record and lets the
 * approval through; only `block` puts the refusal on screen
 * (server/handlers_governance.go), so the seed sets it. `ai-translate` runs
 * in Alice's request context, so every target it wrote is hers, and a plain
 * member carries translate but not review (core/auth
 * DefaultPermissionsForRole). Bob therefore gets the reviewer role scoped to
 * the target locale and writes one target of his own.
 *
 * Idempotent: the policy PUT and Bob's block PUT rewrite the same values, and
 * the project membership is added only when absent. The two block ids are
 * written to harness/.env, because the walk fails its `duties` beat when the
 * refusal does not arrive and only rehearses it when the ids are missing.
 */
async function ensureReviewGovernance(
  ws: string,
  projectId: string,
  aliceToken: string,
  bobToken: string,
): Promise<{ peerBlockId: string; selfBlockId: string }> {
  const sod = await jput<{ mode?: string }>(`/${ws}/sod`, { mode: "block" }, aliceToken);
  console.log(`  · separation of duties: ${sod.mode ?? "block"}`);

  const bobMe = await jget<{ id?: string; user?: { id?: string } }>("/auth/me", bobToken);
  const bobId = bobMe.id || bobMe.user?.id;
  if (!bobId) throw new Error("review governance: no user id for Bob");

  const roles = listOf<Role>(await jget(`/${ws}/roles`, aliceToken), "roles");
  const reviewer =
    roles.find((r) => r.name === "reviewer") ||
    roles.find((r) => (r.permission_names ?? []).includes("review"));
  if (!reviewer) throw new Error("review governance: no reviewer role template");

  const members = listOf<ProjectMember>(await jget(`/${ws}/${projectId}/members`, aliceToken), "members");
  if (members.some((m) => (m.user_id || m.user?.id) === bobId)) {
    console.log(`  · ${BOB.email} already reviews ${COLLAB_LOCALE} on the project`);
  } else {
    await jpost(
      `/${ws}/${projectId}/members`,
      { user_id: bobId, role_id: reviewer.id, languages: [COLLAB_LOCALE] },
      aliceToken,
    );
    console.log(`  · granted ${BOB.email} the reviewer role on ${COLLAB_LOCALE}`);
  }

  const blocks = listOf<Block>(
    await jget(`/${ws}/${projectId}/blocks/main?item=${encodeURIComponent(FILE_NAME)}`, aliceToken),
    "blocks",
  );
  const translatable = blocks.filter((b) => b.translatable !== false);
  const byText = (text: string) => translatable.find((b) => (b.source ?? "").trim() === text);
  const peerBlockId = byText(PEER_BLOCK_SOURCE)?.id ?? "";
  const selfBlockId = byText(SELF_BLOCK_SOURCE)?.id ?? "";
  if (!peerBlockId || !selfBlockId) {
    throw new Error(
      `review governance: ${FILE_NAME} has no block reading "${PEER_BLOCK_SOURCE}" or "${SELF_BLOCK_SOURCE}" ` +
        `(${translatable.length} translatable blocks)`,
    );
  }
  // Bob's PUT is the newest target_modified row for that block and locale, so
  // LastTargetAuthors answers with Bob for it and with Alice for the rest.
  await jput(
    `/${ws}/${projectId}/blocks/main/${peerBlockId}`,
    {
      item_name: FILE_NAME,
      target_locale: COLLAB_LOCALE,
      text: PEER_BLOCK_TARGET,
    },
    bobToken,
  );
  console.log(`  · ${BOB.email} wrote block ${peerBlockId} (${COLLAB_LOCALE})`);
  return { peerBlockId, selfBlockId };
}

interface ConvergenceRun {
  id: string;
  state: string;
  error?: string;
  stall_reason?: string;
}

const estimate = (ws: string, pid: string, token: string) =>
  jget<ConvergenceEstimate>(`/${ws}/${pid}/convergence/estimate`, token);

/**
 * Settle the project's source by running one convergence pass over a locale the
 * seed has already covered.
 *
 * Every run settles the source first (server/convergence_orchestrator.go
 * `runSettleSource`): it stamps each block's SourceStatus and clears the
 * project's `checked` gate. Until something does that, `ready` is 0, every
 * locale's pending count is 0 over the ready source, and the Run-now dialog
 * offers "Transport only" alone. Scoping the run to COLLAB_LOCALE, which the
 * pre-translate step already covered, leaves PENDING_LOCALE untouched, so the
 * settled project still owes a locale the work the walk starts on camera.
 *
 * The alternative, `source_gate: none` on the project, would route around the
 * gate the demo is about.
 */
async function settleSourceWithRun(ws: string, pid: string, token: string): Promise<void> {
  const run = await jpost<ConvergenceRun>(
    `/${ws}/${pid}/convergence/runs`,
    { trigger: "seed", scope: "all", locales: [COLLAB_LOCALE] },
    token,
  );
  console.log(`  · settling source through run ${run.id}`);
  const deadline = Date.now() + 180_000;
  for (;;) {
    const cur = await jget<ConvergenceRun>(`/${ws}/${pid}/convergence/runs/${run.id}`, token);
    if (cur.state !== "running") {
      const why = cur.stall_reason || cur.error;
      console.log(`  · settle run ${cur.state}${why ? ` (${why})` : ""}`);
      return;
    }
    if (Date.now() > deadline) throw new Error(`settle run ${run.id} still running after 180s`);
    await new Promise((res) => setTimeout(res, 2000));
  }
}

/**
 * Clear the pending locale's targets on the recording item, so the walk has
 * work to start whether or not the last take's run already did it.
 *
 * The seed runs immediately before every recording, and the take it precedes
 * translates this locale on camera. Writing an empty target is what the editor
 * does when somebody discards a translation, and a block with no target text is
 * pending again (jobs/decision_basis.go `needsDraft`).
 */
async function clearPendingLocale(ws: string, pid: string, token: string): Promise<number> {
  const blocks = listOf<EditorBlock>(
    await jget(
      `/${ws}/${pid}/blocks/main?item=${encodeURIComponent(FILE_NAME)}&limit=500`,
      token,
    ),
    "blocks",
  );
  const ids = targetsToClear(blocks, PENDING_LOCALE);
  for (const id of ids) {
    await jput(
      `/${ws}/${pid}/blocks/main/${id}`,
      { item_name: FILE_NAME, target_locale: PENDING_LOCALE, text: "" },
      token,
    );
  }
  return ids.length;
}

/**
 * Leave the recording project able to start a real run: source past its gate,
 * and one target locale with pending work.
 *
 * Without both, `ConvergenceRunNowDialog` reads `pending === 0`, offers
 * "Transport only" alone, and the server answers that scope 204 and creates no
 * run, so `[data-testid="run-row"]` never arrives and the automations walk
 * times out (#2597). Both halves are checked rather than assumed: a seed that
 * leaves the project unable to run should say so here rather than in a take.
 */
async function ensureRunnableProject(ws: string, pid: string, token: string): Promise<void> {
  let est = await estimate(ws, pid, token);
  console.log(`  · convergence: ${describeReadiness(est)}`);
  if (needsSettle(est)) {
    await settleSourceWithRun(ws, pid, token);
    est = await estimate(ws, pid, token);
  }
  if (needsPendingWork(est)) {
    const cleared = await clearPendingLocale(ws, pid, token);
    console.log(`  · cleared ${cleared} ${PENDING_LOCALE} target(s) so the walk has work to start`);
    est = await estimate(ws, pid, token);
  }
  if (!isRunnable(est)) {
    throw new Error(
      `the Run-now dialog would offer transport only: ${describeReadiness(est)}`,
    );
  }
  console.log(`  · convergence: ${describeReadiness(est)}, so "Translate all now" starts a real run`);
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

const MEMORY_ENTRIES = [
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

// Workspace terms are concepts (POST /:ws/concepts, server/handlers_concepts.go).
// The direct route creates a term `approved` or `deprecated`; `preferred` and
// `forbidden` are governed statuses it refuses with a 409, because they travel
// through a reviewed change-set.
const CONCEPTS = [
  {
    domain: "cloud",
    definition: "Managed, multi-tenant compute and storage delivered over the network.",
    terms: [
      { text: "cloud infrastructure", locale: "en", status: "approved" },
      { text: "infrastructure cloud", locale: "fr", status: "approved" },
      { text: "Cloud-Infrastruktur", locale: "de", status: "approved" },
    ],
  },
  {
    domain: "reliability",
    definition: "The proportion of time a service is operational and reachable.",
    terms: [
      { text: "uptime", locale: "en", status: "approved" },
      { text: "disponibilité", locale: "fr", status: "approved" },
      { text: "Verfügbarkeit", locale: "de", status: "approved" },
    ],
  },
  {
    domain: "security",
    definition: "Protecting data so only authorised parties can read it, end to end.",
    terms: [
      { text: "encryption", locale: "en", status: "approved" },
      { text: "chiffrement", locale: "fr", status: "approved" },
      { text: "cryptage", locale: "fr", status: "deprecated" },
      { text: "Verschlüsselung", locale: "de", status: "approved" },
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

  await pruneStrayWorkspaces(aliceToken);
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
  // the editor shows target content, and de so the editor walk's locale switch
  // lands on a translated file rather than an empty one. Best-effort (offline
  // demo provider); a re-run re-translates a block only where no target exists.
  for (const target of [COLLAB_LOCALE, "de"]) {
    await jpostSoft(
      `/${ws}/${projectId}/actions/main/ai-translate`,
      { item: FILE_NAME, target_locale: target },
      aliceToken,
    );
  }

  // content memory + terminology: the governance walk (content-memory search
  // "mission", multi-locale concepts) + the editor context panel. Each entry
  // and concept is posted once: the seed runs before every recording so the
  // record tokens stay fresh, and a duplicate row is on camera in the memory
  // list and the concept list.
  await ensureMemoryEntries(ws, projectId, aliceToken);
  await ensureConcepts(ws, projectId, aliceToken);

  // Bob joins (collaboration walk).
  const joined = await ensureMember(ws, aliceToken, bobToken);

  // The review walk's separation-of-duties beats: Bob reviews the target
  // locale and owns one target, so the queue holds both a row Alice may approve
  // and a row the server refuses her.
  const review = await ensureReviewGovernance(ws, projectId, aliceToken, bobToken);

  // The automations walk starts a pass on camera, which needs source past the
  // gate and a locale still owing drafts. Runs last, so it settles the source
  // the pre-translate and the review governance have finished writing.
  await ensureRunnableProject(ws, projectId, aliceToken);

  // Voice profile + Project 2 "Marketing Site" (the correction-loop dropdown
  // needs a SECOND project) + non-compliant content + correction stream.
  const profileId = await ensureBrandProfile(ws, aliceToken, "Acme Voice");
  const marketingId = await ensureProject(ws, aliceToken, "Marketing Site", "en", ["fr", "de"]);
  await uploadIfAbsent(ws, marketingId, aliceToken, "marketing.html", MARKETING_HTML);

  // Guard: only post the correction stream if candidates aren't already there,
  // else re-running multiplies the counts and the demo shows wrong numbers.
  const existingCands = listOf<unknown>(
    await jget(`/${ws}/voice-profiles/${profileId}/candidates?min_count=3`, aliceToken),
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
          `/${ws}/${marketingId}/voice/main/corrections`,
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
    BOWRAIN_ITEM_NAME: FILE_NAME,
    BOWRAIN_COLLAB_LOCALE: COLLAB_LOCALE,
    BOWRAIN_PEER_BLOCK_ID: review.peerBlockId,
    BOWRAIN_SELF_BLOCK_ID: review.selfBlockId,
    BOWRAIN_DEMO_PROFILE_ID: profileId,
  });

  console.log("\n✓ seed complete");
  console.log(`  workspace : ${BASE}/${ws}`);
  console.log(`  project   : Company Website (${projectId}), item ${itemId}`);
  console.log(`  pending   : ${PENDING_LOCALE} (the locale the automations walk translates on camera)`);
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

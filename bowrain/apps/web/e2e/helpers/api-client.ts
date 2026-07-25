/**
 * API client for seeding the real bowrain-server Docker backend.
 * Handles device auth flow, workspace/project CRUD, file upload, content memory, and terminology seeding.
 */
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const BASE_URL =
  process.env.BOWRAIN_SERVER_URL || process.env.BOWRAIN_URL || "http://localhost:8080";
const API = `${BASE_URL}/api/v1`;

export interface SeedContext {
  token: string;
  workspaceSlug: string;
  workspaceId: string;
}

// --- Authentication ---

/** Perform the device auth flow and return a JWT access token.
 *  If BOWRAIN_TOKEN is set, returns it directly (for external server mode). */
export async function authenticate(
  email = "admin@example.com",
  name = "Demo User",
): Promise<string> {
  // Fast path: pre-supplied token for external server mode.
  const preSupplied = process.env.BOWRAIN_TOKEN;
  if (preSupplied) return preSupplied;

  // Step 1: Start device auth
  const startResp = await fetch(`${API}/auth/device/start`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: "client_id=e2e-screenshots",
  });
  if (!startResp.ok) throw new Error(`Device start failed: ${startResp.status}`);
  const startData = await startResp.json();
  const { device_code, user_code } = startData;

  // Step 2: Verify (simulates user approving in browser)
  // Use redirect: "manual" because the handler responds with 302 to a SPA page.
  const verifyResp = await fetch(`${API}/auth/device/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `user_code=${user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
    redirect: "manual",
  });
  if (!verifyResp.ok && verifyResp.status !== 302)
    throw new Error(`Device verify failed: ${verifyResp.status}`);

  // Step 3: Poll for token
  const pollResp = await fetch(`${API}/auth/device/poll`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `device_code=${device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
  });
  if (!pollResp.ok) throw new Error(`Device poll failed: ${pollResp.status}`);
  const pollData = await pollResp.json();
  const token = pollData.access_token;

  // Step 4: Complete onboarding (idempotent). A real user goes through the
  // onboarding screen after first sign-in, which creates their PERSONAL
  // workspace — project claiming (POST /projects/claim) requires it.
  await ensureOnboarded(token);

  return token;
}

/** Complete onboarding for the token's user if needed (creates the personal
 *  workspace). Idempotent: if the user already onboarded, the server returns
 *  the existing personal workspace regardless of the requested slug. */
export async function ensureOnboarded(token: string): Promise<void> {
  const stateResp = await fetch(`${API}/auth/me/onboarding`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!stateResp.ok) throw new Error(`GET onboarding state failed: ${stateResp.status}`);
  const state = await stateResp.json();
  if (!state.needs_onboarding) return;

  // The suggestion can be rejected server-side (e.g. "admin" derived from
  // admin@example.com is a reserved word), so fall back to a fixed slug.
  const candidates = [state.suggested_slug, "e2e-demo-user"].filter(Boolean) as string[];
  let lastError = "";
  for (const slug of candidates) {
    const resp = await fetch(`${API}/auth/me/onboarding`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ slug }),
    });
    if (resp.ok) return;
    lastError = `${resp.status} ${await resp.text()}`;
  }
  throw new Error(`Complete onboarding failed: ${lastError}`);
}

// --- Helpers ---

async function apiGet(path: string, token: string) {
  const resp = await fetch(`${API}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!resp.ok) throw new Error(`GET ${path} failed: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

async function apiPost(path: string, token: string, body?: unknown) {
  const resp = await fetch(`${API}${path}`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!resp.ok) throw new Error(`POST ${path} failed: ${resp.status} ${await resp.text()}`);
  if (resp.status === 204) return null;
  return resp.json();
}

async function apiDelete(path: string, token: string) {
  const resp = await fetch(`${API}${path}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!resp.ok && resp.status !== 204) throw new Error(`DELETE ${path} failed: ${resp.status}`);
}

// --- Workspace ---

export async function createWorkspace(
  token: string,
  name: string,
  slug: string,
): Promise<{ id: string; slug: string }> {
  const ws = await apiPost("/workspaces", token, { name, slug });
  return { id: ws.id, slug: ws.slug };
}

export async function getOrCreateWorkspace(
  token: string,
  name: string,
  slug: string,
): Promise<{ id: string; slug: string }> {
  // GET first; only attempt create on a true 404. Any other failure
  // (403 — exists but not yours, 5xx — server error) should propagate
  // rather than silently colliding via duplicate-slug INSERT.
  const resp = await fetch(`${API}/${slug}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (resp.ok) {
    const ws = await resp.json();
    return { id: ws.id, slug: ws.slug };
  }
  if (resp.status !== 404) {
    throw new Error(`GET /${slug} failed: ${resp.status} ${await resp.text()}`);
  }
  return createWorkspace(token, name, slug);
}

// --- Editor Projects ---
// Routes follow Bowrain AD-011's flat scheme: /:ws/projects (collection) and
// /:ws/:projectId (project) — the same endpoints the web app's RestApiAdapter
// uses (bowrain/packages/ui/src/api/rest-adapter.ts).

export async function createEditorProject(
  token: string,
  wsSlug: string,
  name: string,
  sourceLocale: string,
  targetLocales: string[],
): Promise<{ id: string; name: string }> {
  const p = await apiPost(`/${wsSlug}/projects`, token, {
    name,
    default_source_language: sourceLocale,
    target_languages: targetLocales,
  });
  return { id: p.id, name: p.name };
}

export async function listEditorProjects(
  token: string,
  wsSlug: string,
): Promise<Array<{ id: string; name: string }>> {
  return apiGet(`/${wsSlug}/projects`, token);
}

/** Fetch a single project with its items (needed to resolve item IDs for URLs). */
export async function getEditorProject(
  token: string,
  wsSlug: string,
  projectId: string,
): Promise<{ id: string; name: string; items: Array<{ id: string; name: string }> }> {
  return apiGet(`/${wsSlug}/${projectId}`, token);
}

/** Find an item ID by filename within a project's items array. */
export function findItemId(
  project: { items: Array<{ id: string; name: string }> },
  fileName: string,
): string {
  const item = project.items.find((i) => i.name === fileName);
  if (!item) throw new Error(`Item not found: ${fileName}`);
  return item.id;
}

export async function deleteEditorProject(
  token: string,
  wsSlug: string,
  projectId: string,
): Promise<void> {
  await apiDelete(`/${wsSlug}/${projectId}`, token);
}

/** Delete all editor projects in the workspace. */
export async function deleteAllEditorProjects(token: string, wsSlug: string): Promise<void> {
  const projects = await listEditorProjects(token, wsSlug);
  if (!projects) return;
  for (const p of projects) {
    await deleteEditorProject(token, wsSlug, p.id);
  }
}

// --- File Operations ---

export async function uploadFile(
  token: string,
  wsSlug: string,
  projectId: string,
  filePath: string,
): Promise<void> {
  const fileName = path.basename(filePath);
  const fileContent = fs.readFileSync(filePath);
  const formData = new FormData();
  formData.append("files", new Blob([fileContent]), fileName);

  const resp = await fetch(`${API}/${wsSlug}/${projectId}/items/main`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: formData,
  });
  if (!resp.ok) throw new Error(`Upload ${fileName} failed: ${resp.status} ${await resp.text()}`);
}

export async function uploadSeedFiles(
  token: string,
  wsSlug: string,
  projectId: string,
  fileNames: string[],
): Promise<void> {
  const seedDir = path.resolve(__dirname, "../seed");
  for (const name of fileNames) {
    await uploadFile(token, wsSlug, projectId, path.join(seedDir, name));
  }
}

// --- Translation Operations ---

export async function pseudoTranslateFile(
  token: string,
  wsSlug: string,
  projectId: string,
  fileName: string,
  targetLocale: string,
): Promise<{ total_blocks: number; translated_blocks: number }> {
  return apiPost(`/${wsSlug}/${projectId}/actions/main/pseudo-translate`, token, {
    item: fileName,
    target_locale: targetLocale,
  });
}

// --- content memory Seeding ---

interface Entry {
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
}

export async function seedMemoryEntries(
  token: string,
  wsSlug: string,
  entriesPath?: string,
): Promise<number> {
  const filePath = entriesPath || path.resolve(__dirname, "../seed/tm-entries.json");
  const entries: Entry[] = JSON.parse(fs.readFileSync(filePath, "utf-8"));
  for (const entry of entries) {
    await apiPost(`/${wsSlug}/translation-memory`, token, entry);
  }
  return entries.length;
}

// --- Terminology Seeding ---

interface ConceptTerm {
  text: string;
  locale: string;
  status?: string;
  part_of_speech?: string;
  gender?: string;
}

interface Concept {
  domain: string;
  definition: string;
  terms: ConceptTerm[];
}

export async function seedConcepts(
  token: string,
  wsSlug: string,
  conceptsPath?: string,
): Promise<number> {
  const filePath = conceptsPath || path.resolve(__dirname, "../seed/concepts.json");
  const concepts: Concept[] = JSON.parse(fs.readFileSync(filePath, "utf-8"));
  for (const concept of concepts) {
    await apiPost(`/${wsSlug}/concepts`, token, concept);
  }
  return concepts.length;
}

// --- Invitations ---

export async function createInvite(
  token: string,
  wsSlug: string,
  role: string,
  email?: string,
  maxUses?: number,
  ttlDays?: number,
): Promise<Invite> {
  const body: Record<string, unknown> = { role };
  if (email) body.email = email;
  if (maxUses !== undefined) body.max_uses = maxUses;
  if (ttlDays !== undefined) body.ttl_days = ttlDays;
  return apiPost(`/${wsSlug}/invites`, token, body);
}

export async function listInvites(token: string, wsSlug: string): Promise<Invite[]> {
  return apiGet(`/${wsSlug}/invites`, token);
}

export async function acceptInvite(token: string, code: string): Promise<void> {
  await apiPost(`/join/${code}`, token);
}

interface Invite {
  id: string;
  code: string;
  email: string;
  role: string;
  max_uses: number;
  use_count: number;
  expires_at: string;
}

// --- Anonymous Projects & Claim ---

export async function createAnonymousProject(
  name: string,
  sourceLocale: string,
  targetLocales: string[],
): Promise<{ project_id: string; claim_token: string }> {
  const resp = await fetch(`${API}/projects/anonymous`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name,
      default_source_language: sourceLocale,
      target_languages: targetLocales,
    }),
  });
  if (!resp.ok)
    throw new Error(`Create anonymous project failed: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

export async function claimProject(
  token: string,
  claimToken: string,
): Promise<{ project_id: string; workspace_slug: string }> {
  return apiPost("/projects/claim", token, { claim_token: claimToken });
}

// --- Full Seed ---

export interface SeedResult {
  context: SeedContext;
  projects: Array<{ id: string; name: string; files: string[] }>;
  memoryCount: number;
  conceptCount: number;
}

/**
 * Performs a complete seed of the Docker backend:
 * 1. Authenticates via device auth
 * 2. Creates workspace "Acme Inc." (slug: acme)
 * 3. Creates projects with uploaded files
 * 4. Seeds content-memory entries and terminology concepts
 */
export async function fullSeed(): Promise<SeedResult> {
  console.log("Authenticating...");
  const token = await authenticate();

  console.log("Creating workspace...");
  const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");

  console.log("Creating projects and uploading files...");
  const projects: SeedResult["projects"] = [];

  // Project 1: Website (HTML)
  const p1 = await createEditorProject(token, ws.slug, "Company Website", "en", ["fr", "de"]);
  await uploadSeedFiles(token, ws.slug, p1.id, ["about-us.html"]);
  projects.push({ id: p1.id, name: p1.name, files: ["about-us.html"] });

  // Project 2: Mobile App (JSON)
  const p2 = await createEditorProject(token, ws.slug, "Mobile App", "en", ["fr", "de"]);
  await uploadSeedFiles(token, ws.slug, p2.id, ["app-strings.json"]);
  projects.push({ id: p2.id, name: p2.name, files: ["app-strings.json"] });

  // Project 3: Release Notes (Markdown)
  const p3 = await createEditorProject(token, ws.slug, "Release Notes", "en", ["fr"]);
  await uploadSeedFiles(token, ws.slug, p3.id, ["release-notes.md"]);
  projects.push({ id: p3.id, name: p3.name, files: ["release-notes.md"] });

  console.log("Seeding content-memory entries...");
  const memoryCount = await seedMemoryEntries(token, ws.slug);

  console.log("Seeding terminology concepts...");
  const conceptCount = await seedConcepts(token, ws.slug);

  console.log(
    `Seed complete: ${projects.length} projects, ${memoryCount} content-memory entries, ${conceptCount} concepts`,
  );

  return {
    context: { token, workspaceSlug: ws.slug, workspaceId: ws.id },
    projects,
    memoryCount,
    conceptCount,
  };
}

/** Component status from the readiness endpoint. */
export interface ReadinessComponentStatus {
  status: string;
  type?: string;
  latency_ms?: number;
  providers?: Array<{ name: string; model?: string; configured: boolean }>;
  error?: string;
}

/** Full readiness response from /api/v1/ready. */
export interface ReadinessInfo {
  status: "ready" | "degraded" | "unhealthy";
  version: string;
  components: Record<string, ReadinessComponentStatus>;
}

/**
 * Wait for the server to be ready (all critical components up).
 * Returns the readiness info so callers can inspect component status
 * (e.g. check `components.ai.status` to skip AI-dependent tests).
 */
export async function waitForReady(maxWaitMs = 60000): Promise<ReadinessInfo> {
  const start = Date.now();
  let lastError: string | undefined;
  while (Date.now() - start < maxWaitMs) {
    try {
      const resp = await fetch(`${API}/ready`);
      if (resp.ok || resp.status === 503) {
        const info: ReadinessInfo = await resp.json();
        if (info.status !== "unhealthy") return info;
        // Accept unhealthy if core components work (AI/email may not be configured).
        // SQLite mode reports database as "unconfigured" (no pgDB) but is functional.
        const dbStatus = info.components?.database?.status;
        if (dbStatus === "up" || dbStatus === "unconfigured") return info;
        lastError = `status=${info.status}`;
      }
    } catch {
      // Server not ready yet
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error(`Server not ready after ${maxWaitMs}ms (${lastError ?? "unreachable"})`);
}

/** Wait for the server to be healthy (retry with backoff).
 *  @deprecated Use waitForReady() for richer status information. */
export async function waitForServer(maxWaitMs = 60000): Promise<void> {
  await waitForReady(maxWaitMs);
}

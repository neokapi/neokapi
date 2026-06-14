/**
 * API client for seeding the real bowrain-server backend for desktop recordings.
 * Ported from bowrain/apps/web/e2e/helpers/api-client.ts.
 * Handles device auth flow, workspace/project CRUD, file upload, TM, and terminology seeding.
 */
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const BASE_URL = process.env.BOWRAIN_SERVER_URL || "http://localhost:8080";
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

  const startResp = await fetch(`${API}/auth/device/start`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: "client_id=e2e-desktop",
  });
  if (!startResp.ok) throw new Error(`Device start failed: ${startResp.status}`);
  const startData = await startResp.json();
  const { device_code, user_code } = startData;

  const verifyResp = await fetch(`${API}/auth/device/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `user_code=${user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
    redirect: "manual",
  });
  if (!verifyResp.ok && verifyResp.status !== 302)
    throw new Error(`Device verify failed: ${verifyResp.status}`);

  const pollResp = await fetch(`${API}/auth/device/poll`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `device_code=${device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
  });
  if (!pollResp.ok) throw new Error(`Device poll failed: ${pollResp.status}`);
  const pollData = await pollResp.json();
  return pollData.access_token;
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
  try {
    const ws = await apiGet(`/workspaces/${slug}`, token);
    return { id: ws.id, slug: ws.slug };
  } catch {
    return createWorkspace(token, name, slug);
  }
}

// --- Editor Projects ---

export async function createEditorProject(
  token: string,
  wsSlug: string,
  name: string,
  sourceLocale: string,
  targetLocales: string[],
): Promise<{ id: string; name: string }> {
  const p = await apiPost(`/workspaces/${wsSlug}/editor/projects`, token, {
    name,
    source_locale: sourceLocale,
    target_locales: targetLocales,
  });
  return { id: p.id, name: p.name };
}

export async function listEditorProjects(
  token: string,
  wsSlug: string,
): Promise<Array<{ id: string; name: string }>> {
  return apiGet(`/workspaces/${wsSlug}/editor/projects`, token);
}

export async function deleteEditorProject(
  token: string,
  wsSlug: string,
  projectId: string,
): Promise<void> {
  await apiDelete(`/workspaces/${wsSlug}/editor/projects/${projectId}`, token);
}

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

  const resp = await fetch(`${API}/workspaces/${wsSlug}/editor/projects/${projectId}/files`, {
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
  // Reuse web app's seed files
  const seedDir = path.resolve(__dirname, "../../../../web/e2e/seed");
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
  return apiPost(
    `/workspaces/${wsSlug}/editor/projects/${projectId}/file-pseudo/${encodeURIComponent(fileName)}`,
    token,
    { target_locale: targetLocale },
  );
}

// --- TM Seeding ---

interface TMEntry {
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
}

export async function seedTMEntries(
  token: string,
  wsSlug: string,
  entriesPath?: string,
): Promise<number> {
  const filePath =
    entriesPath || path.resolve(__dirname, "../../../../web/e2e/seed/tm-entries.json");
  const entries: TMEntry[] = JSON.parse(fs.readFileSync(filePath, "utf-8"));
  for (const entry of entries) {
    await apiPost(`/workspaces/${wsSlug}/tm`, token, entry);
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
  const filePath =
    conceptsPath || path.resolve(__dirname, "../../../../web/e2e/seed/concepts.json");
  const concepts: Concept[] = JSON.parse(fs.readFileSync(filePath, "utf-8"));
  for (const concept of concepts) {
    await apiPost(`/workspaces/${wsSlug}/concepts`, token, concept);
  }
  return concepts.length;
}

// --- Full Seed ---

export interface SeedResult {
  context: SeedContext;
  projects: Array<{ id: string; name: string; files: string[] }>;
  tmCount: number;
  conceptCount: number;
}

/**
 * Performs a complete seed of the Docker backend:
 * 1. Authenticates via device auth
 * 2. Creates workspace "Acme Inc." (slug: acme)
 * 3. Creates projects with uploaded files
 * 4. Seeds TM entries and terminology concepts
 */
export async function fullSeed(): Promise<SeedResult> {
  console.log("Authenticating...");
  const token = await authenticate();

  console.log("Creating workspace...");
  const slug = `desktop-${Date.now().toString(36)}`;
  const ws = await getOrCreateWorkspace(token, "Acme Inc.", slug);

  console.log("Creating projects and uploading files...");
  const projects: SeedResult["projects"] = [];

  const p1 = await createEditorProject(token, ws.slug, "Company Website", "en", ["fr", "de"]);
  await uploadSeedFiles(token, ws.slug, p1.id, ["about-us.html"]);
  projects.push({ id: p1.id, name: p1.name, files: ["about-us.html"] });

  const p2 = await createEditorProject(token, ws.slug, "Mobile App", "en", ["fr", "de"]);
  await uploadSeedFiles(token, ws.slug, p2.id, ["app-strings.json"]);
  projects.push({ id: p2.id, name: p2.name, files: ["app-strings.json"] });

  const p3 = await createEditorProject(token, ws.slug, "Release Notes", "en", ["fr"]);
  await uploadSeedFiles(token, ws.slug, p3.id, ["release-notes.md"]);
  projects.push({ id: p3.id, name: p3.name, files: ["release-notes.md"] });

  console.log("Seeding TM entries...");
  const tmCount = await seedTMEntries(token, ws.slug);

  console.log("Seeding terminology concepts...");
  const conceptCount = await seedConcepts(token, ws.slug);

  console.log(
    `Seed complete: ${projects.length} projects, ${tmCount} TM entries, ${conceptCount} concepts`,
  );

  return {
    context: { token, workspaceSlug: ws.slug, workspaceId: ws.id },
    projects,
    tmCount,
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

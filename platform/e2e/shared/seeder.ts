/**
 * Layered seeder for the "Acme CloudOps" e2e story.
 *
 * Each layer builds on the previous one and is designed to be idempotent
 * where the API supports it (check-before-create).
 *
 * Layers:
 *   1. Foundation  — workspace + projects + file uploads
 *   2. Language     — TM entries + terminology concepts
 *   3. Brand Voice  — brand profile from starter pack
 *   4. Collaboration — stream + tasks
 *   5. Automation   — automation rules
 */

import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

import { BowrainAPI } from "./api-client.js";
import {
  WORKSPACE,
  PROJECTS,
  TM_ENTRIES,
  CONCEPTS,
  BRAND_PROFILE,
  TASKS,
  AUTOMATION_RULES,
  STREAM,
} from "./seed-data.js";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface StoryContext {
  api: BowrainAPI;
  wsSlug: string;
  projects: Record<string, { id: string; name: string }>;
  brandProfileId?: string;
  stream?: string;
  taskIds?: string[];
  tmCount: number;
  conceptCount: number;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function seedFilesDir(): string {
  return path.resolve(__dirname, "seed-files");
}

async function uploadSeedFile(
  api: BowrainAPI,
  wsSlug: string,
  projectId: string,
  fileName: string,
): Promise<void> {
  const filePath = path.join(seedFilesDir(), fileName);
  const content = fs.readFileSync(filePath);
  await api.uploadFile(wsSlug, projectId, fileName, content);
}

// ---------------------------------------------------------------------------
// Layer 1: Foundation — workspace, projects, files
// ---------------------------------------------------------------------------

export async function seedFoundation(api: BowrainAPI): Promise<StoryContext> {
  const suffix = Date.now().toString(36);
  const slug = `${WORKSPACE.slugPrefix}-${suffix}`;

  console.log(`[seed] Creating workspace "${WORKSPACE.name}" (${slug})...`);
  const ws = await api.getOrCreateWorkspace(WORKSPACE.name, slug);

  const projects: Record<string, { id: string; name: string }> = {};

  for (const def of PROJECTS) {
    console.log(`[seed]   Project "${def.name}" (${def.sourceLanguage} -> ${def.targetLanguages.join(", ")})...`);

    // Check if project already exists (idempotent).
    const existing = await api.listProjects(ws.slug);
    const found = existing.find((p) => p.name === def.name);
    let projectId: string;

    if (found) {
      projectId = found.id;
      console.log(`[seed]     Already exists (${projectId})`);
    } else {
      const created = await api.createProject(
        ws.slug,
        def.name,
        def.sourceLanguage,
        def.targetLanguages,
      );
      projectId = created.id;
      console.log(`[seed]     Created (${projectId})`);
    }

    // Upload files.
    for (const fileName of def.files) {
      console.log(`[seed]     Uploading ${fileName}...`);
      await uploadSeedFile(api, ws.slug, projectId, fileName);
    }

    projects[def.name] = { id: projectId, name: def.name };
  }

  console.log(`[seed] Foundation complete: ${Object.keys(projects).length} projects`);
  return { api, wsSlug: ws.slug, projects, tmCount: 0, conceptCount: 0 };
}

// ---------------------------------------------------------------------------
// Layer 2: Language Assets — TM entries + terminology
// ---------------------------------------------------------------------------

export async function seedLanguageAssets(ctx: StoryContext): Promise<StoryContext> {
  const { api, wsSlug } = ctx;

  console.log(`[seed] Seeding ${TM_ENTRIES.length} TM entries...`);
  for (const entry of TM_ENTRIES) {
    await api.addTMEntry(wsSlug, entry.source, entry.target, entry.source_locale, entry.target_locale);
  }

  console.log(`[seed] Seeding ${CONCEPTS.length} terminology concepts...`);
  for (const concept of CONCEPTS) {
    await api.addConcept(wsSlug, concept);
  }

  console.log(`[seed] Language assets complete`);
  return {
    ...ctx,
    tmCount: TM_ENTRIES.length,
    conceptCount: CONCEPTS.length,
  };
}

// ---------------------------------------------------------------------------
// Layer 3: Brand Voice — brand profile from starter pack
// ---------------------------------------------------------------------------

export async function seedBrandVoice(ctx: StoryContext): Promise<StoryContext> {
  const { api, wsSlug } = ctx;

  console.log(`[seed] Creating brand profile from starter pack "${BRAND_PROFILE.pack}"...`);

  // Check if a profile with the same name already exists.
  const existing = await api.listBrandProfiles(wsSlug);
  const found = existing.find((p) => p.name === BRAND_PROFILE.name);

  if (found) {
    console.log(`[seed]   Brand profile already exists (${found.id})`);
    return { ...ctx, brandProfileId: found.id };
  }

  const profile = await api.createBrandProfileFromStarter(
    wsSlug,
    BRAND_PROFILE.pack,
    BRAND_PROFILE.name,
  );
  console.log(`[seed]   Created brand profile (${profile.id})`);
  return { ...ctx, brandProfileId: profile.id };
}

// ---------------------------------------------------------------------------
// Layer 4: Collaboration — stream + tasks
// ---------------------------------------------------------------------------

export async function seedCollaboration(ctx: StoryContext): Promise<StoryContext> {
  const { api, wsSlug, projects } = ctx;

  // Create a stream on the first project (Company Website).
  const websiteProject = projects["Company Website"];
  let streamId: string | undefined;

  if (websiteProject) {
    console.log(`[seed] Creating stream "${STREAM.name}" on "${websiteProject.name}"...`);
    try {
      const existing = await api.listStreams(wsSlug, websiteProject.id);
      const found = existing.find((s) => s.name === STREAM.name);
      if (found) {
        streamId = found.id;
        console.log(`[seed]   Stream already exists (${streamId})`);
      } else {
        const stream = await api.createStream(wsSlug, websiteProject.id, {
          name: STREAM.name,
          description: STREAM.description,
        });
        streamId = stream.id;
        console.log(`[seed]   Created stream (${streamId})`);
      }
    } catch (err) {
      console.log(`[seed]   Stream creation skipped (${err instanceof Error ? err.message : err})`);
    }
  }

  // Create tasks.
  const taskIds: string[] = [];
  console.log(`[seed] Creating ${TASKS.length} tasks...`);

  for (const def of TASKS) {
    const taskBody: Record<string, unknown> = {
      title: def.title,
    };
    if (def.description) taskBody.description = def.description;
    if (def.type) taskBody.type = def.type;
    if (def.priority) taskBody.priority = def.priority;

    // Associate with project if specified.
    if (def.projectIndex !== undefined) {
      const projDef = PROJECTS[def.projectIndex];
      const proj = projDef ? projects[projDef.name] : undefined;
      if (proj) taskBody.project_id = proj.id;
    }

    try {
      const task = await api.createTask(wsSlug, taskBody as Parameters<typeof api.createTask>[1]);
      taskIds.push(task.id);
      console.log(`[seed]   Task "${def.title}" (${task.id})`);
    } catch (err) {
      console.log(`[seed]   Task creation failed: ${err instanceof Error ? err.message : err}`);
    }
  }

  console.log(`[seed] Collaboration complete: ${taskIds.length} tasks`);
  return { ...ctx, stream: streamId, taskIds };
}

// ---------------------------------------------------------------------------
// Layer 5: Automation — automation rules
// ---------------------------------------------------------------------------

export async function seedAutomation(ctx: StoryContext): Promise<StoryContext> {
  const { api, wsSlug, projects } = ctx;

  console.log(`[seed] Creating ${AUTOMATION_RULES.length} automation rules...`);

  for (const def of AUTOMATION_RULES) {
    const projDef = PROJECTS[def.projectIndex];
    const proj = projDef ? projects[projDef.name] : undefined;
    if (!proj) {
      console.log(`[seed]   Skipping rule "${def.name}" — project not found`);
      continue;
    }

    try {
      // Check for existing rule with same name.
      const existing = await api.listAutomationRules(wsSlug, proj.id);
      const found = existing.find((r) => r.name === def.name);
      if (found) {
        console.log(`[seed]   Rule "${def.name}" already exists (${found.id})`);
        continue;
      }

      const rule = await api.createAutomationRule(wsSlug, proj.id, {
        name: def.name,
        trigger: def.trigger,
        conditions: def.conditions,
        actions: def.actions,
        enabled: def.enabled,
      });
      console.log(`[seed]   Rule "${def.name}" (${rule.id})`);
    } catch (err) {
      console.log(`[seed]   Rule creation failed: ${err instanceof Error ? err.message : err}`);
    }
  }

  console.log(`[seed] Automation complete`);
  return ctx;
}

// ---------------------------------------------------------------------------
// Full seed — all layers in sequence
// ---------------------------------------------------------------------------

export async function fullSeed(api: BowrainAPI): Promise<StoryContext> {
  console.log("[seed] Starting full seed...");
  let ctx = await seedFoundation(api);
  ctx = await seedLanguageAssets(ctx);
  ctx = await seedBrandVoice(ctx);
  ctx = await seedCollaboration(ctx);
  ctx = await seedAutomation(ctx);
  console.log(
    `[seed] Full seed complete: ws=${ctx.wsSlug}, ` +
    `${Object.keys(ctx.projects).length} projects, ` +
    `${ctx.tmCount} TM entries, ` +
    `${ctx.conceptCount} concepts, ` +
    `${ctx.taskIds?.length ?? 0} tasks`,
  );
  return ctx;
}

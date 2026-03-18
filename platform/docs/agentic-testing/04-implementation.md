# Technical Implementation

## Technology Choices

### Option A: Go Service (Recommended)

Build the orchestrator and agent runtime in Go, co-located with neokapi.

**Pros:**
- Same language as Bowrain — shared types, direct CLI integration
- Can import Bowrain's API client, auth, and project packages directly
- Agents can invoke bowrain CLI as a subprocess or use the Go API directly
- Fits the existing monorepo structure (`neokapi/agentic/`)
- Strong concurrency primitives (goroutines, channels) match the agent model

**Cons:**
- LLM SDK integration less mature in Go (but Anthropic has a Go SDK)
- Agent prompting logic is string-heavy (less ergonomic than Python/TS)

### Option B: TypeScript Service

Build on the existing e2e test infrastructure (Playwright + TypeScript).

**Pros:**
- Existing BowrainAPI client, Keycloak admin, and seeder already written
- Playwright for browser automation (Web UI agent interactions)
- Rich LLM SDK ecosystem (Anthropic SDK, LangChain, etc.)
- Agent prompts naturally expressed as template literals

**Cons:**
- Separate from the Go codebase (can't share types)
- CLI interactions require subprocess calls anyway
- Another runtime dependency

### Option C: Hybrid (Recommended for Phase 1)

TypeScript orchestrator (leveraging existing e2e infrastructure) + Go CLI wrapper.

**Pros:**
- Reuse BowrainAPI client and auth from `platform/e2e/shared/`
- Playwright for Web UI interactions (Brand Manager, PM, Translator agents)
- Call bowrain CLI via subprocess for Developer agent
- LLM calls via Anthropic TypeScript SDK
- Fastest path to MVP

**Decision:** Start with **Option C** (TypeScript hybrid), evaluate migration to pure Go (Option A) once patterns stabilize.

## Repository Structure

```
neokapi/agentic/
├── package.json
├── tsconfig.json
├── config/
│   ├── global.yaml              # Global orchestration config
│   └── projects/
│       ├── docusaurus.yaml      # Per-project config
│       ├── gitea.yaml
│       ├── home-assistant.yaml
│       └── tolgee.yaml
├── src/
│   ├── orchestrator/
│   │   ├── index.ts             # Main entry point
│   │   ├── scheduler.ts         # Cron + event-driven scheduling
│   │   ├── state.ts             # SQLite state manager
│   │   ├── dispatcher.ts        # Agent launcher + result collector
│   │   ├── event-router.ts      # Event → agent trigger routing
│   │   └── config.ts            # Config loading + validation
│   ├── agents/
│   │   ├── base-agent.ts        # Shared agent interface + LLM integration
│   │   ├── developer.ts         # Developer agent (CLI + git)
│   │   ├── brand-manager.ts     # Brand Manager agent (Web UI + API)
│   │   ├── translator.ts        # Translator agent (Web UI + API)
│   │   ├── pm.ts                # Project Manager agent (Web UI + API)
│   │   └── qa.ts                # QA agent (CLI + API)
│   ├── llm/
│   │   ├── client.ts            # Anthropic SDK wrapper
│   │   ├── prompts.ts           # Prompt templates per persona
│   │   └── tools.ts             # Tool definitions for agent function calling
│   ├── platform/
│   │   ├── api-client.ts        # Extended from e2e shared (reuse!)
│   │   ├── cli-wrapper.ts       # bowrain CLI subprocess wrapper
│   │   ├── git-ops.ts           # Git operations (clone, merge, branch, PR)
│   │   └── browser.ts           # Playwright browser automation
│   ├── monitors/
│   │   ├── upstream.ts          # Watch upstream repos for new releases
│   │   ├── metrics.ts           # Collect and expose metrics
│   │   └── screenshots.ts      # Periodic screenshot capture
│   └── utils/
│       ├── logger.ts            # Structured logging
│       ├── timing.ts            # Jitter, delays, work windows
│       └── cost-tracker.ts      # AI spend monitoring
├── data/
│   ├── state.db                 # SQLite state database
│   └── screenshots/             # Captured screenshots
└── docs/                        # → symlink to docs/agentic-testing/
```

## Agent Runtime

### Base Agent

Every agent shares a common runtime that provides LLM integration, state management, and platform access.

```typescript
// src/agents/base-agent.ts
interface AgentContext {
  project: ProjectConfig;
  identity: PersonaIdentity;
  state: AgentState;
  api: BowrainAPI;
  llm: AnthropicClient;
  logger: Logger;
}

interface AgentTask {
  name: string;
  description: string;
  execute(ctx: AgentContext): Promise<AgentResult>;
}

interface AgentResult {
  status: "completed" | "partial" | "blocked" | "error";
  blocksProcessed: number;
  actionsPerformed: Action[];
  nextTask?: string;           // Suggest what to do next
  events: AgentEvent[];        // Events to emit
}

abstract class BaseAgent {
  protected ctx: AgentContext;

  // Run a single work session
  async runSession(task: AgentTask): Promise<AgentResult> {
    this.ctx.logger.info(`Starting session: ${task.name}`);
    this.ctx.state.status = "working";

    try {
      const result = await task.execute(this.ctx);
      this.ctx.state.sessionCount++;
      this.ctx.state.blocksProcessed += result.blocksProcessed;
      this.ctx.state.lastSessionAt = new Date();
      return result;
    } catch (error) {
      this.ctx.state.status = "error";
      throw error;
    }
  }

  // Ask the LLM for a decision
  protected async decide(prompt: string, tools: Tool[]): Promise<LLMResponse> {
    return this.ctx.llm.createMessage({
      model: "claude-sonnet-4-5-20250514",
      system: this.getSystemPrompt(),
      messages: [{ role: "user", content: prompt }],
      tools,
      max_tokens: 4096,
    });
  }
}
```

### Developer Agent

```typescript
// src/agents/developer.ts
class DeveloperAgent extends BaseAgent {
  private cli: CLIWrapper;
  private git: GitOps;

  tasks = {
    check_upstream_and_push: {
      name: "check_upstream_and_push",
      description: "Check for upstream changes and push new content",
      execute: async (ctx: AgentContext) => {
        // 1. Check for new upstream releases/commits
        const updates = await this.git.checkUpstream(ctx.project);
        if (!updates.hasChanges) {
          return { status: "completed" as const, blocksProcessed: 0, events: [], actionsPerformed: [] };
        }

        // 2. Merge upstream changes
        await this.git.mergeUpstream(updates.ref);

        // 3. Push to Bowrain
        const pushResult = await this.cli.push();

        // 4. Emit event for downstream agents
        return {
          status: "completed" as const,
          blocksProcessed: pushResult.blockCount,
          actionsPerformed: [{ type: "push", detail: pushResult }],
          events: [{ type: "content_pushed", data: pushResult }],
        };
      },
    },

    pull_translations: {
      name: "pull_translations",
      description: "Pull completed translations and commit",
      execute: async (ctx: AgentContext) => {
        const pullResult = await this.cli.pull({ allLocales: true });
        if (pullResult.filesChanged > 0) {
          await this.git.commit(`l10n: pull translations for ${ctx.project.name}`);
          await this.git.push();
        }
        return {
          status: "completed" as const,
          blocksProcessed: pullResult.blockCount,
          actionsPerformed: [{ type: "pull", detail: pullResult }],
          events: [{ type: "translations_pulled", data: pullResult }],
        };
      },
    },

    create_stream: {
      name: "create_stream",
      description: "Create a Bowrain stream for a new version",
      execute: async (ctx: AgentContext) => {
        // LLM decides stream name and parent based on context
        const decision = await this.decide(
          `New release ${ctx.state.currentRelease} detected. ` +
          `Current streams: ${ctx.state.streams.join(", ")}. ` +
          `Should I create a new stream? If so, what name and parent?`,
          [tools.createStream, tools.skipStream]
        );
        // Execute the LLM's tool call
        return this.executeTool(decision);
      },
    },
  };
}
```

### Translator Agent

```typescript
// src/agents/translator.ts
class TranslatorAgent extends BaseAgent {
  tasks = {
    translate_assigned_batch: {
      name: "translate_assigned_batch",
      description: "Translate a batch of assigned blocks",
      execute: async (ctx: AgentContext) => {
        // 1. Get assigned tasks
        const tasks = await ctx.api.listTasks(ctx.project.workspaceSlug, {
          assignee: ctx.identity.email,
          status: "open",
        });

        if (tasks.length === 0) {
          return { status: "completed" as const, blocksProcessed: 0, events: [], actionsPerformed: [] };
        }

        // 2. For each task, get blocks to translate
        let totalProcessed = 0;
        for (const task of tasks.slice(0, ctx.config.blocksPerSession)) {
          // 3. Get AI translation suggestion
          const aiTranslation = await ctx.api.pseudoTranslate(
            ctx.project.workspaceSlug,
            task.projectId,
            task.fileName,
            ctx.identity.targetLanguage
          );

          // 4. LLM reviews AI translation (persona-specific critique)
          const review = await this.decide(
            `Review this AI translation from English to ${ctx.identity.targetLanguage}:\n` +
            `Source: "${task.sourceText}"\n` +
            `AI Translation: "${aiTranslation}"\n` +
            `Termbase entries: ${ctx.state.relevantTerms}\n` +
            `Brand guidelines: ${ctx.state.brandProfile}\n` +
            `Should I accept, edit, or reject this translation?`,
            [tools.acceptTranslation, tools.editTranslation, tools.rejectTranslation]
          );

          // 5. Apply decision
          await this.executeTranslationDecision(review, task);
          totalProcessed++;

          // 6. Occasionally add to TM (high-confidence translations)
          if (review.confidence > 0.9) {
            await ctx.api.addTMEntry(
              ctx.project.workspaceSlug,
              task.sourceText,
              review.finalTranslation,
              "en-US",
              ctx.identity.targetLanguage
            );
          }
        }

        return {
          status: totalProcessed >= ctx.config.blocksPerSession ? "partial" as const : "completed" as const,
          blocksProcessed: totalProcessed,
          actionsPerformed: [{ type: "translate", detail: { count: totalProcessed } }],
          events: [{ type: "translation_batch_complete", data: { count: totalProcessed } }],
        };
      },
    },
  };
}
```

### Brand Manager Agent

```typescript
// src/agents/brand-manager.ts
class BrandManagerAgent extends BaseAgent {
  tasks = {
    review_terminology: {
      name: "review_terminology",
      description: "Review and update project terminology",
      execute: async (ctx: AgentContext) => {
        // 1. Get recent content pushes
        const activities = await ctx.api.listActivities(ctx.project.workspaceSlug, {
          type: "content_push",
          since: ctx.state.lastSessionAt,
        });

        // 2. Extract candidate terms using LLM
        const candidates = await this.decide(
          `Review these recently pushed blocks for terminology candidates:\n` +
          `${activities.map((a: any) => a.summary).join("\n")}\n\n` +
          `Current termbase has ${ctx.state.termCount} concepts.\n` +
          `Identify new terms that should be standardized.`,
          [tools.addConcept, tools.updateConcept, tools.skipTerm]
        );

        // 3. Apply terminology updates
        const results = await this.executeTerminologyDecisions(candidates);

        return {
          status: "completed" as const,
          blocksProcessed: 0,
          events: [{ type: "terminology_updated", data: results }],
          actionsPerformed: results.actions,
        };
      },
    },

    create_brand_profile: {
      name: "create_brand_profile",
      description: "Create or update brand voice profile",
      execute: async (ctx: AgentContext) => {
        // LLM analyzes project content and creates brand profile
        const analysis = await this.decide(
          `Analyze the content style of ${ctx.project.name} and create a brand profile.\n` +
          `Project type: ${ctx.project.category}\n` +
          `Sample content: ${ctx.state.sampleBlocks}\n` +
          `Choose from starter packs: tech, enterprise, casual, academic\n` +
          `Then customize tone, style, and vocabulary.`,
          [tools.createBrandProfile, tools.updateBrandProfile]
        );

        return this.executeBrandDecision(analysis);
      },
    },
  };
}
```

## LLM Integration

### Tool Definitions

Agents use Claude's tool-use capability to make decisions and take actions:

```typescript
// src/llm/tools.ts
const tools = {
  // Developer tools
  createStream: {
    name: "create_stream",
    description: "Create a new Bowrain stream for a release or feature",
    input_schema: {
      type: "object",
      properties: {
        name: { type: "string", description: "Stream name (e.g., 'v3.2')" },
        parent: { type: "string", description: "Parent stream name" },
        description: { type: "string" },
      },
      required: ["name"],
    },
  },

  // Translator tools
  acceptTranslation: {
    name: "accept_translation",
    description: "Accept the AI translation as-is",
    input_schema: {
      type: "object",
      properties: {
        confidence: { type: "number", minimum: 0, maximum: 1 },
        reason: { type: "string" },
      },
      required: ["confidence"],
    },
  },

  editTranslation: {
    name: "edit_translation",
    description: "Edit the AI translation before accepting",
    input_schema: {
      type: "object",
      properties: {
        edited_text: { type: "string" },
        changes_made: { type: "string", description: "What was changed and why" },
        confidence: { type: "number" },
      },
      required: ["edited_text", "confidence"],
    },
  },

  // Brand Manager tools
  addConcept: {
    name: "add_concept",
    description: "Add a new terminology concept to the termbase",
    input_schema: {
      type: "object",
      properties: {
        term: { type: "string" },
        definition: { type: "string" },
        domain: { type: "string", enum: ["software", "ui", "marketing", "legal"] },
        status: { type: "string", enum: ["preferred", "approved", "deprecated"] },
      },
      required: ["term", "definition", "domain"],
    },
  },

  // PM tools
  createTask: {
    name: "create_task",
    description: "Create a translation task and assign to a team member",
    input_schema: {
      type: "object",
      properties: {
        title: { type: "string" },
        assignee: { type: "string" },
        priority: { type: "string", enum: ["low", "medium", "high", "urgent"] },
        deadline: { type: "string", format: "date" },
        scope: { type: "string", description: "Files or blocks to translate" },
      },
      required: ["title", "assignee", "priority"],
    },
  },
};
```

### Prompt Engineering

System prompts are composed from:

1. **Base persona** (from `01-agent-personas.md` templates)
2. **Project context** (name, languages, current state)
3. **Session context** (what happened since last session, pending items)
4. **Constraints** (budget, time window, quality targets)

```typescript
// src/llm/prompts.ts
function buildSystemPrompt(ctx: AgentContext): string {
  const parts = [
    getPersonaPrompt(ctx.identity),
    getProjectContext(ctx.project, ctx.state),
    getSessionContext(ctx.state),
    getConstraints(ctx.config),
  ];
  return parts.join("\n\n---\n\n");
}

function getSessionContext(state: AgentState): string {
  return `
## Current Session Context

Last session: ${state.lastSessionAt?.toISOString() ?? "never"}
Sessions completed: ${state.sessionCount}
Blocks processed total: ${state.blocksProcessed}
Current pending items: ${state.pendingItems.length}
Recent team activity:
${state.recentActivity.map((a: any) => `- ${a.agent}: ${a.action}`).join("\n")}
`;
}
```

## Browser Automation Layer

For agents that interact with the Web UI (Brand Manager, Translator, PM):

```typescript
// src/platform/browser.ts
class BowrainBrowser {
  private page: Page;

  // Navigate to translation editor
  async openTranslationEditor(wsSlug: string, projectId: string, fileName: string) {
    await this.page.goto(`/${wsSlug}/translate?project=${projectId}&file=${fileName}`);
    await this.page.waitForSelector("[data-testid='translation-editor']");
  }

  // Edit a translation block
  async editTranslation(blockIndex: number, text: string) {
    const block = this.page.locator(`[data-testid='block-${blockIndex}'] .target-cell`);
    await block.click();
    await block.fill(text);
    // Wait for auto-save
    await this.page.waitForSelector("[data-testid='save-indicator'][data-status='saved']");
  }

  // Open brand dashboard
  async openBrandDashboard(wsSlug: string) {
    await this.page.goto(`/${wsSlug}/brand-dashboard`);
    await this.page.waitForSelector("[data-testid='brand-profiles']");
  }

  // Create brand profile from starter
  async createBrandFromStarter(starter: string, name: string) {
    await this.page.click(`[data-testid='starter-${starter}']`);
    await this.page.fill("[data-testid='profile-name']", name);
    await this.page.click("[data-testid='create-profile']");
  }

  // Screenshot for demo material
  async captureScreenshot(name: string): Promise<string> {
    const path = `data/screenshots/${name}-${Date.now()}.png`;
    await this.page.screenshot({ path, fullPage: true });
    return path;
  }
}
```

## CLI Wrapper

Uses safe subprocess execution (no shell interpretation):

```typescript
// src/platform/cli-wrapper.ts
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

class CLIWrapper {
  private cwd: string;  // Project fork directory

  async init(server: string, source: string, targets: string[]): Promise<void> {
    await this.run("bowrain", ["init", "--server", server, "--source", source, "--targets", targets.join(",")]);
  }

  async push(): Promise<PushResult> {
    const output = await this.run("bowrain", ["push", "--json"]);
    return JSON.parse(output);
  }

  async pull(opts?: { locale?: string; allLocales?: boolean }): Promise<PullResult> {
    const args = ["pull", "--json"];
    if (opts?.locale) args.push("--locale", opts.locale);
    if (opts?.allLocales) args.push("--all");
    const output = await this.run("bowrain", args);
    return JSON.parse(output);
  }

  async sync(opts?: { timeout?: string }): Promise<SyncResult> {
    const args = ["sync", "--json"];
    if (opts?.timeout) args.push("--timeout", opts.timeout);
    const output = await this.run("bowrain", args);
    return JSON.parse(output);
  }

  async status(): Promise<StatusResult> {
    const output = await this.run("bowrain", ["status", "--json"]);
    return JSON.parse(output);
  }

  async streamCreate(name: string, parent?: string): Promise<void> {
    const args = ["stream", "create", name];
    if (parent) args.push("--parent", parent);
    await this.run("bowrain", args);
  }

  private async run(command: string, args: string[]): Promise<string> {
    const { stdout } = await execFileAsync(command, args, { cwd: this.cwd });
    return stdout;
  }
}
```

## Git Operations

```typescript
// src/platform/git-ops.ts
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

class GitOps {
  private repoPath: string;

  async checkUpstream(project: ProjectConfig): Promise<UpstreamUpdate> {
    // Fetch upstream
    await this.git("fetch", ["upstream"]);

    // Check for new tags
    const { stdout: tags } = await this.git("tag", ["--list", "v*", "--sort=-version:refname"]);
    const latestTag = tags.split("\n")[0];

    // Check if we've already processed this tag
    const { stdout: currentRef } = await this.git("rev-parse", ["HEAD"]);

    return {
      hasChanges: latestTag !== project.state.upstreamRef,
      ref: latestTag,
      currentRef: currentRef.trim(),
    };
  }

  async mergeUpstream(ref: string): Promise<void> {
    await this.git("merge", [`upstream/${ref}`, "--no-edit"]);
  }

  async commit(message: string): Promise<string> {
    await this.git("add", ["-A"]);
    const { stdout } = await this.git("commit", ["-m", message]);
    return stdout;
  }

  async push(): Promise<void> {
    await this.git("push", ["origin", "HEAD"]);
  }

  async createBranch(name: string): Promise<void> {
    await this.git("checkout", ["-b", name]);
  }

  private async git(subcommand: string, args: string[]): Promise<{ stdout: string; stderr: string }> {
    return execFileAsync("git", [subcommand, ...args], { cwd: this.repoPath });
  }
}
```

## Infrastructure

### Local Development

```bash
# Prerequisites
brew install node@22
npm install -g pnpm

# Setup
cd neokapi/agentic
pnpm install

# Run with local Bowrain server
BOWRAIN_URL=http://localhost:8080 pnpm start

# Run single agent for testing
pnpm run agent -- --project docusaurus --persona developer --task check_upstream
```

### Production Deployment

**Option A: Long-Running Service**
- Deployed as a container (Docker/Podman)
- Runs on a dedicated VM or Kubernetes pod
- SQLite state persists in a volume
- Connects to production Bowrain server

**Option B: Scheduled GitHub Actions (Lightweight)**
- Each agent session is a GitHub Actions workflow run
- State stored in a git repo or artifact
- Cheaper (pay per minute), but slower to iterate

**Option C: Cron on a Dev Machine**
- Simplest for development/demo
- `crontab` triggers the orchestrator periodically
- State in local SQLite

**Recommendation:** Start with Option C for development, move to Option A for sustained demos.

### Environment Setup

```yaml
# docker-compose.yaml for the full stack
services:
  bowrain-server:
    image: bowrain/server:latest
    ports: ["8080:8080"]
    environment:
      DATABASE_URL: postgres://...

  keycloak:
    image: quay.io/keycloak/keycloak:latest
    ports: ["8180:8080"]

  agentic-orchestrator:
    build: ./neokapi/agentic
    depends_on: [bowrain-server, keycloak]
    environment:
      BOWRAIN_URL: http://bowrain-server:8080
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
    volumes:
      - ./data:/app/data           # State persistence
      - ./forks:/app/forks         # Git forks
```

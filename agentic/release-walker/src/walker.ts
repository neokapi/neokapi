import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { writeFile, readFile, unlink } from "node:fs/promises";
import { join } from "node:path";
import { Redis } from "ioredis";
type RedisClient = Redis;
import { Octokit } from "@octokit/rest";
import type { ProjectConfig } from "./config.js";

const execFileAsync = promisify(execFile);

/** Options for the release walk loop. */
export interface WalkOptions {
  /** Minutes between releases (default: 120). */
  intervalMinutes: number;
  /** Maximum releases to process per day (default: 6). */
  maxPerDay: number;
  /** Tag to start walking from (inclusive). Omit to start from earliest. */
  startRelease?: string;
  /** Base directory where fork repos are cloned. */
  forksDir: string;
  /** Redis URL for pub/sub signaling. */
  redisUrl?: string;
  /** GitHub token for API access. */
  githubToken?: string;
}

/** Marker file written to the fork workspace after merging a release. */
const MARKER_FILE = ".zeroclaw-release-ready";

/** State file tracking which releases have been processed. */
const STATE_FILE = ".release-walker-state.json";

interface WalkerState {
  lastProcessedTag: string | null;
  processedTags: string[];
}

/**
 * Get sorted release tags from the upstream repository.
 * Uses GitHub API if a token is available, otherwise falls back to git.
 */
export async function getReleases(
  project: ProjectConfig,
  forkPath: string,
  githubToken?: string,
): Promise<string[]> {
  if (githubToken) {
    return getReleasesFromGitHub(project.upstream, githubToken);
  }
  return getReleasesFromGit(forkPath);
}

async function getReleasesFromGitHub(
  upstream: string,
  token: string,
): Promise<string[]> {
  const octokit = new Octokit({ auth: token });
  const [owner, repo] = upstream.split("/");

  const tags: string[] = [];
  for await (const response of octokit.paginate.iterator(
    octokit.rest.repos.listTags,
    { owner, repo, per_page: 100 },
  )) {
    for (const tag of response.data) {
      tags.push(tag.name);
    }
  }

  // Sort by version (semver-like sorting via git's version:refname logic).
  // Tags come from GitHub unsorted; we sort them lexicographically with
  // version-aware comparison.
  return tags.sort(compareVersionTags);
}

async function getReleasesFromGit(forkPath: string): Promise<string[]> {
  // Fetch upstream tags first.
  await execFileAsync("git", ["fetch", "upstream", "--tags"], {
    cwd: forkPath,
  });

  const { stdout } = await execFileAsync(
    "git",
    ["tag", "--list", "v*", "--sort=version:refname"],
    { cwd: forkPath },
  );

  return stdout
    .trim()
    .split("\n")
    .filter((t) => t.length > 0);
}

/**
 * Compare two version tags for sorting.
 * Handles v-prefixed semver tags (v1.2.3, v1.2.3-rc.1).
 */
function compareVersionTags(a: string, b: string): number {
  const parseVersion = (tag: string) => {
    const match = tag.match(
      /^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$/,
    );
    if (!match) return null;
    return {
      major: parseInt(match[1], 10),
      minor: parseInt(match[2], 10),
      patch: parseInt(match[3], 10),
      prerelease: match[4] || "",
    };
  };

  const va = parseVersion(a);
  const vb = parseVersion(b);

  if (!va && !vb) return a.localeCompare(b);
  if (!va) return 1;
  if (!vb) return -1;

  if (va.major !== vb.major) return va.major - vb.major;
  if (va.minor !== vb.minor) return va.minor - vb.minor;
  if (va.patch !== vb.patch) return va.patch - vb.patch;

  // No prerelease is "greater" than any prerelease (1.0.0 > 1.0.0-rc.1).
  if (!va.prerelease && vb.prerelease) return 1;
  if (va.prerelease && !vb.prerelease) return -1;
  return va.prerelease.localeCompare(vb.prerelease);
}

/**
 * Merge an upstream release tag into the fork's working branch.
 */
export async function mergeRelease(
  forkPath: string,
  tag: string,
): Promise<void> {
  // Fetch the specific tag from upstream.
  await execFileAsync("git", ["fetch", "upstream", `refs/tags/${tag}`], {
    cwd: forkPath,
  });

  // Merge the tag into the current branch.
  await execFileAsync(
    "git",
    ["merge", `upstream/${tag}`, "--no-edit", "-m", `Merge upstream ${tag}`],
    { cwd: forkPath },
  );
}

/**
 * Signal agents that a new release has been merged and is ready for processing.
 * Prefers Redis pub/sub; falls back to a marker file.
 */
export async function signalAgents(
  project: ProjectConfig,
  tag: string,
  forkPath: string,
  redis?: RedisClient,
): Promise<void> {
  const message = JSON.stringify({
    project: project.name,
    upstream: project.upstream,
    fork: project.fork,
    tag,
    timestamp: new Date().toISOString(),
  });

  if (redis) {
    await redis.publish("agentic:content-pushed", message);
    console.log(`  Published to agentic:content-pushed: ${project.name}@${tag}`);
  }

  // Always write the marker file as well (belt and suspenders).
  const markerPath = join(forkPath, MARKER_FILE);
  await writeFile(markerPath, message, "utf-8");
  console.log(`  Wrote marker file: ${markerPath}`);
}

/**
 * Wait for agents to finish processing a release.
 * Listens on Redis channel `agentic:qa-passed` for a matching completion message.
 * Falls back to polling the marker file removal if Redis is unavailable.
 */
export async function waitForCompletion(
  project: ProjectConfig,
  tag: string,
  forkPath: string,
  timeoutMs: number,
  redisUrl?: string,
): Promise<boolean> {
  if (redisUrl) {
    return waitForCompletionRedis(project, tag, timeoutMs, redisUrl);
  }
  return waitForCompletionMarker(forkPath, timeoutMs);
}

async function waitForCompletionRedis(
  project: ProjectConfig,
  tag: string,
  timeoutMs: number,
  redisUrl: string,
): Promise<boolean> {
  const subscriber = new Redis(redisUrl);

  try {
    return await new Promise<boolean>((resolve) => {
      const timer = setTimeout(() => {
        console.log(`  Timeout waiting for completion of ${project.name}@${tag}`);
        subscriber.unsubscribe("agentic:qa-passed");
        resolve(false);
      }, timeoutMs);

      subscriber.subscribe("agentic:qa-passed").catch((err: unknown) => {
        console.error("  Failed to subscribe to agentic:qa-passed:", err);
        clearTimeout(timer);
        resolve(false);
      });

      subscriber.on("message", (_channel: string, message: string) => {
        try {
          const data = JSON.parse(message);
          if (data.project === project.name && data.tag === tag) {
            console.log(`  Completion confirmed for ${project.name}@${tag}`);
            clearTimeout(timer);
            subscriber.unsubscribe("agentic:qa-passed");
            resolve(true);
          }
        } catch {
          // Ignore malformed messages.
        }
      });
    });
  } finally {
    subscriber.disconnect();
  }
}

async function waitForCompletionMarker(
  forkPath: string,
  timeoutMs: number,
): Promise<boolean> {
  const markerPath = join(forkPath, MARKER_FILE);
  const pollInterval = 30_000; // 30 seconds
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    try {
      await readFile(markerPath);
      // Marker still exists — agents haven't finished.
      await new Promise((r) => setTimeout(r, pollInterval));
    } catch {
      // Marker removed — agents are done.
      console.log("  Marker file removed — processing complete.");
      return true;
    }
  }

  console.log("  Timeout waiting for marker file removal.");
  // Clean up the marker so it doesn't block the next release.
  try {
    await unlink(markerPath);
  } catch {
    // Already gone — fine.
  }
  return false;
}

/**
 * Load the walker state for a project (tracks which releases have been processed).
 */
async function loadState(forkPath: string): Promise<WalkerState> {
  const statePath = join(forkPath, STATE_FILE);
  try {
    const raw = await readFile(statePath, "utf-8");
    return JSON.parse(raw) as WalkerState;
  } catch {
    return { lastProcessedTag: null, processedTags: [] };
  }
}

/**
 * Save the walker state for a project.
 */
async function saveState(
  forkPath: string,
  state: WalkerState,
): Promise<void> {
  const statePath = join(forkPath, STATE_FILE);
  await writeFile(statePath, JSON.stringify(state, null, 2), "utf-8");
}

/**
 * Walk through releases for a single project.
 * This is the main loop — it fetches tags, filters to the relevant range,
 * merges each one, signals agents, waits for completion, and paces.
 */
export async function walkProject(
  project: ProjectConfig,
  options: WalkOptions,
): Promise<void> {
  const forkPath = join(options.forksDir, project.name);
  console.log(`\n=== Walking releases for ${project.name} ===`);
  console.log(`  Upstream: ${project.upstream}`);
  console.log(`  Fork path: ${forkPath}`);

  // Load state for idempotent resume.
  const state = await loadState(forkPath);

  // Get all release tags.
  const allTags = await getReleases(project, forkPath, options.githubToken);
  console.log(`  Found ${allTags.length} release tags`);

  if (allTags.length === 0) {
    console.log("  No release tags found. Skipping.");
    return;
  }

  // Determine the starting point.
  const startTag = options.startRelease || state.lastProcessedTag;
  let tags = allTags;
  if (startTag) {
    const startIdx = allTags.indexOf(startTag);
    if (startIdx >= 0) {
      // If resuming, skip the already-processed tag.
      const offset = state.processedTags.includes(startTag) ? 1 : 0;
      tags = allTags.slice(startIdx + offset);
    } else {
      console.log(
        `  Warning: start release "${startTag}" not found in tag list. Starting from beginning.`,
      );
    }
  }

  // Filter out already-processed tags (for idempotent resume).
  tags = tags.filter((t) => !state.processedTags.includes(t));

  if (tags.length === 0) {
    console.log("  All releases already processed. Nothing to do.");
    return;
  }

  console.log(`  Processing ${tags.length} releases: ${tags[0]} -> ${tags[tags.length - 1]}`);

  // Connect to Redis if available.
  let redis: RedisClient | undefined;
  if (options.redisUrl) {
    try {
      redis = new Redis(options.redisUrl);
      console.log("  Connected to Redis for signaling.");
    } catch (err) {
      console.log("  Redis unavailable, falling back to marker files:", err);
    }
  }

  let processedToday = 0;
  let dayStart = Date.now();

  try {
    for (const tag of tags) {
      // Enforce daily limit.
      const elapsed = Date.now() - dayStart;
      if (elapsed > 24 * 60 * 60 * 1000) {
        // New day — reset counter.
        processedToday = 0;
        dayStart = Date.now();
      }
      if (processedToday >= options.maxPerDay) {
        console.log(
          `  Reached daily limit of ${options.maxPerDay} releases. Pausing until next day.`,
        );
        const msUntilNextDay = 24 * 60 * 60 * 1000 - elapsed;
        await new Promise((r) => setTimeout(r, msUntilNextDay));
        processedToday = 0;
        dayStart = Date.now();
      }

      console.log(`\n--- Release: ${tag} (${processedToday + 1}/${options.maxPerDay} today) ---`);

      // Merge the upstream tag.
      try {
        await mergeRelease(forkPath, tag);
        console.log(`  Merged upstream ${tag}`);
      } catch (err) {
        console.error(`  Failed to merge ${tag}:`, err);
        console.log("  Skipping this release.");
        continue;
      }

      // Signal agents.
      await signalAgents(project, tag, forkPath, redis);

      // Wait for processing to complete.
      const completionTimeout = options.intervalMinutes * 60 * 1000;
      const completed = await waitForCompletion(
        project,
        tag,
        forkPath,
        completionTimeout,
        options.redisUrl,
      );
      if (!completed) {
        console.log(`  Warning: processing did not complete within timeout for ${tag}`);
      }

      // Update state.
      state.lastProcessedTag = tag;
      state.processedTags.push(tag);
      await saveState(forkPath, state);
      processedToday++;

      // Pace — wait before next release (skip on last).
      if (tag !== tags[tags.length - 1]) {
        const waitMs = options.intervalMinutes * 60 * 1000;
        console.log(
          `  Pacing: waiting ${options.intervalMinutes} minutes before next release...`,
        );
        await new Promise((r) => setTimeout(r, waitMs));
      }
    }
  } finally {
    if (redis) {
      redis.disconnect();
    }
  }

  console.log(`\n=== Completed walking ${project.name} ===`);
}

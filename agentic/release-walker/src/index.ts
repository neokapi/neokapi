import { parseArgs } from "node:util";
import { loadProjectConfigs } from "./config.js";
import { walkProject, type WalkOptions } from "./walker.js";

const DEFAULT_INTERVAL_MINUTES = 120;
const DEFAULT_MAX_PER_DAY = 6;
const DEFAULT_CONFIG_DIR = process.env.CONFIG_DIR || "/app/config";
const DEFAULT_FORKS_DIR = process.env.FORKS_DIR || "/app/forks";

function usage(): never {
  console.log(`
Usage: release-walker [options]

Options:
  --project <name>         Walk a specific project (default: all projects)
  --start-release <tag>    Start from this release tag (default: earliest or resume point)
  --interval <minutes>     Minutes between releases (default: ${DEFAULT_INTERVAL_MINUTES})
  --max-per-day <n>        Maximum releases per day (default: ${DEFAULT_MAX_PER_DAY})
  --config-dir <path>      Path to config directory (default: ${DEFAULT_CONFIG_DIR})
  --forks-dir <path>       Path to forks directory (default: ${DEFAULT_FORKS_DIR})
  --help                   Show this help message

Environment variables:
  REDIS_URL                Redis URL for pub/sub signaling (e.g. redis://redis:6379)
  GITHUB_TOKEN             GitHub token for API access (recommended)
  CONFIG_DIR               Config directory (overridden by --config-dir)
  FORKS_DIR                Forks directory (overridden by --forks-dir)

Examples:
  # Walk all projects from their earliest tracked release
  release-walker --interval 60

  # Walk a specific project from a tag
  release-walker --project docusaurus --start-release v3.0.0

  # Quick walkthrough with short intervals
  release-walker --project excalidraw --interval 30 --max-per-day 12
`);
  process.exit(0);
}

async function main(): Promise<void> {
  const { values } = parseArgs({
    options: {
      project: { type: "string" },
      "start-release": { type: "string" },
      interval: { type: "string" },
      "max-per-day": { type: "string" },
      "config-dir": { type: "string" },
      "forks-dir": { type: "string" },
      help: { type: "boolean" },
    },
    strict: true,
  });

  if (values.help) {
    usage();
  }

  const configDir = values["config-dir"] || DEFAULT_CONFIG_DIR;
  const forksDir = values["forks-dir"] || DEFAULT_FORKS_DIR;
  const intervalMinutes = values.interval
    ? parseInt(values.interval, 10)
    : DEFAULT_INTERVAL_MINUTES;
  const maxPerDay = values["max-per-day"]
    ? parseInt(values["max-per-day"], 10)
    : DEFAULT_MAX_PER_DAY;

  if (isNaN(intervalMinutes) || intervalMinutes <= 0) {
    console.error("Error: --interval must be a positive number");
    process.exit(1);
  }
  if (isNaN(maxPerDay) || maxPerDay <= 0) {
    console.error("Error: --max-per-day must be a positive number");
    process.exit(1);
  }

  console.log("Release Walker starting...");
  console.log(`  Config dir: ${configDir}`);
  console.log(`  Forks dir: ${forksDir}`);
  console.log(`  Interval: ${intervalMinutes} minutes`);
  console.log(`  Max per day: ${maxPerDay}`);
  console.log(`  Redis: ${process.env.REDIS_URL || "(not configured)"}`);
  console.log(`  GitHub token: ${process.env.GITHUB_TOKEN ? "(set)" : "(not set)"}`);

  // Load project configs.
  const projects = await loadProjectConfigs(configDir, values.project);
  console.log(
    `  Projects: ${projects.map((p) => p.name).join(", ")}`,
  );

  const walkOptions: WalkOptions = {
    intervalMinutes,
    maxPerDay,
    startRelease: values["start-release"],
    forksDir,
    redisUrl: process.env.REDIS_URL,
    githubToken: process.env.GITHUB_TOKEN,
  };

  // Walk each project sequentially (they share the pacing budget).
  for (const project of projects) {
    try {
      await walkProject(project, walkOptions);
    } catch (err) {
      console.error(`Error walking project ${project.name}:`, err);
      // Continue with next project.
    }
  }

  console.log("\nRelease Walker finished.");
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});

import { readFile, readdir } from "node:fs/promises";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";

/** Parsed project configuration from YAML. */
export interface ProjectConfig {
  /** Project name derived from the YAML filename (e.g. "docusaurus"). */
  name: string;
  /** Upstream GitHub repository (e.g. "facebook/docusaurus"). */
  upstream: string;
  /** Fork GitHub repository (e.g. "neokapi/agentic-docusaurus"). */
  fork: string;
  /** Source language code. */
  sourceLanguage: string;
  /** Target language codes. */
  targetLanguages: string[];
  /** Content path patterns. */
  contentPaths: ContentPath[];
}

export interface ContentPath {
  path: string;
  format?: string;
}

/** Raw shape of the project YAML files. */
interface RawProjectYaml {
  upstream: string;
  fork: string;
  source_language: string;
  target_languages: string[];
  content_paths: (string | { path: string; format?: string })[];
}

/**
 * Load a single project config from a YAML file.
 */
export async function loadProjectConfig(
  filePath: string,
): Promise<ProjectConfig> {
  const raw = await readFile(filePath, "utf-8");
  const parsed = parseYaml(raw) as RawProjectYaml;

  const name = filePath
    .split("/")
    .pop()!
    .replace(/\.yaml$/, "");

  const contentPaths: ContentPath[] = parsed.content_paths.map((cp) => {
    if (typeof cp === "string") {
      return { path: cp };
    }
    return { path: cp.path, format: cp.format };
  });

  return {
    name,
    upstream: parsed.upstream,
    fork: parsed.fork,
    sourceLanguage: parsed.source_language,
    targetLanguages: parsed.target_languages,
    contentPaths,
  };
}

/**
 * Load all project configs from the config/projects/ directory.
 * If `projectName` is specified, load only that project.
 */
export async function loadProjectConfigs(
  configDir: string,
  projectName?: string,
): Promise<ProjectConfig[]> {
  const projectsDir = join(configDir, "projects");
  const files = await readdir(projectsDir);
  const yamlFiles = files.filter((f) => f.endsWith(".yaml"));

  const configs: ProjectConfig[] = [];
  for (const file of yamlFiles) {
    const name = file.replace(/\.yaml$/, "");
    if (projectName && name !== projectName) {
      continue;
    }
    const config = await loadProjectConfig(join(projectsDir, file));
    configs.push(config);
  }

  if (projectName && configs.length === 0) {
    throw new Error(
      `Project "${projectName}" not found in ${projectsDir}. ` +
        `Available: ${yamlFiles.map((f) => f.replace(/\.yaml$/, "")).join(", ")}`,
    );
  }

  return configs;
}

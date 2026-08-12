/**
 * The Remotion composition registry: one row per authored demo.
 *
 * Both fields come from demos/<id>/demo.yaml, the committed source of truth, and
 * nothing here reads public/. The generated module is tracked, so it may depend
 * only on tracked inputs: a machine that has captured every demo and one that
 * has captured none must write the same file, or a partial run rewrites it to
 * match the local capture tree and deletes rows it was never asked to touch.
 */
import fs from "node:fs";
import path from "node:path";
import { listDemoIds, loadManifest } from "../lib/manifest.ts";
import { DEMOS_DIR, HARNESS_ROOT, PUBLIC_DIR, ensureDir } from "../lib/paths.ts";

export interface RegistryEntry {
  id: string;
  title: string;
}

/** Every authored demo, id-sorted, titled as its manifest titles it. */
export function registryEntries(demosDir: string = DEMOS_DIR): RegistryEntry[] {
  return listDemoIds(demosDir).map((id) => ({ id, title: loadManifest(id, demosDir).title }));
}

/** The generated TypeScript module the Remotion Root imports. */
export function registrySource(entries: RegistryEntry[]): string {
  return `// Auto-generated from demos/*/demo.yaml by src/cli/run.ts. Do not edit by hand.
export interface RegistryEntry { id: string; title: string; }
export const DEMOS: RegistryEntry[] = ${JSON.stringify(entries, null, 2)};
`;
}

/** Write the generated module, plus the copy Studio serves from public/. */
export function writeRegistry(demosDir: string = DEMOS_DIR): RegistryEntry[] {
  const entries = registryEntries(demosDir);
  fs.writeFileSync(path.join(HARNESS_ROOT, "src", "remotion", "registry.generated.ts"), registrySource(entries));
  ensureDir(PUBLIC_DIR);
  fs.writeFileSync(path.join(PUBLIC_DIR, "registry.json"), JSON.stringify(entries, null, 2));
  return entries;
}

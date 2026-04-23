/**
 * Builds `translations-manifest.json` from the bundler's output graph.
 *
 * Pairs up two inputs:
 *   - hashesByFile  → every hash each source file emitted into a
 *                     `__t` / `__tx` call (collected during transform).
 *   - bundle        → Rollup/Vite's output chunk set; each chunk lists
 *                     the module IDs it was built from.
 *
 * Intersection of those two produces the per-chunk hash set. The
 * runtime uses the manifest (via `kapi-react split` or a custom
 * pipeline) to serve only the strings a given JS chunk actually
 * needs, so lazy routes don't force a full-catalog download.
 *
 * See the issue #406 design for the overall story.
 */

export interface TranslationsManifest {
  /**
   * Schema version. Bumps when the shape changes in a non-additive
   * way so consumers can bail cleanly on mismatch. Additive changes
   * (new optional fields) do NOT bump the version.
   */
  version: 1;
  /**
   * chunk name → sorted array of hashes used by that chunk's
   * modules. Stable ordering (sorted) so the file is diff-friendly
   * and reproducible builds stay reproducible.
   */
  chunks: Record<string, string[]>;
}

/**
 * Minimal chunk shape the manifest builder needs. Matches Rollup's
 * `OutputChunk` without pulling in the full Rollup types (keeps this
 * module testable under plain Vitest without bundler deps).
 */
export interface ChunkLike {
  type: "chunk";
  name: string;
  modules: Record<string, unknown>;
}

export interface AssetLike {
  type: "asset";
}

export type BundleLike = Record<string, ChunkLike | AssetLike>;

/**
 * Builds the manifest by unioning each chunk's module hashes. Chunks
 * with no translatable modules are omitted. Hash arrays are sorted so
 * two identical builds produce byte-identical manifests.
 */
export function buildChunkManifest(
  bundle: BundleLike,
  hashesByFile: Map<string, Set<string>>,
): TranslationsManifest {
  const chunks: Record<string, string[]> = {};

  for (const entry of Object.values(bundle)) {
    if (entry.type !== "chunk") continue;
    const chunkHashes = new Set<string>();
    for (const moduleId of Object.keys(entry.modules)) {
      const fileHashes = hashesByFile.get(moduleId);
      if (!fileHashes) continue;
      for (const h of fileHashes) chunkHashes.add(h);
    }
    if (chunkHashes.size === 0) continue;
    // Stable ordering — sorted keeps diffs readable and builds reproducible.
    const name = entry.name;
    const existing = chunks[name];
    if (existing) {
      // Two chunks with the same `name` (rare but legal in Rollup —
      // e.g. dynamic imports that share a name). Union them.
      for (const h of chunkHashes) existing.push(h);
      chunks[name] = Array.from(new Set(existing)).sort();
    } else {
      chunks[name] = Array.from(chunkHashes).sort();
    }
  }

  return { version: 1, chunks };
}

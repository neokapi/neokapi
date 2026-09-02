// "2 hours ago" and "13.4 KB": the two formatters a resource listing needs.
//
// The relative one lives in `lib/when.ts` beside the absolute rendering it
// shares an `Intl` locale with, and is re-exported here so the browser's own
// components keep one import. A surface that wants the instant rather than the
// distance draws a `When`.
export { relativeTime } from "../../lib/when";

/** Format bytes as a human-readable size string. */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

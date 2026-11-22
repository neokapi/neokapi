import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll } from "vite-plus/test";

// Every directory tempDir makes in a test file is removed once the file's
// tests have run, so a test run leaves nothing in the system temporary
// directory.
const made: string[] = [];

afterAll(() => {
  for (const dir of made.splice(0)) rmSync(dir, { recursive: true, force: true });
});

/** tempDir makes a fresh directory under the system temporary directory. */
export function tempDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), `${prefix}-`));
  made.push(dir);
  return dir;
}

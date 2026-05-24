import { spawn, type SpawnOptions } from "node:child_process";

export interface RunResult {
  code: number | null;
  stdout: string;
  stderr: string;
  timedOut: boolean;
}

/** Run a command, capturing stdout/stderr, with an optional timeout (ms). */
export function run(
  cmd: string,
  args: string[],
  opts: SpawnOptions & { timeoutMs?: number; input?: string; onStdout?: (chunk: string) => void } = {},
): Promise<RunResult> {
  const { timeoutMs, input, onStdout, ...spawnOpts } = opts;
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, { ...spawnOpts, stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let timer: NodeJS.Timeout | undefined;

    child.stdout?.on("data", (d) => {
      const s = d.toString();
      stdout += s;
      onStdout?.(s);
    });
    child.stderr?.on("data", (d) => {
      stderr += d.toString();
    });
    child.on("error", (err) => {
      if (timer) clearTimeout(timer);
      reject(err);
    });
    child.on("close", (code) => {
      if (timer) clearTimeout(timer);
      resolve({ code, stdout, stderr, timedOut });
    });

    if (timeoutMs) {
      timer = setTimeout(() => {
        timedOut = true;
        child.kill("SIGTERM");
        setTimeout(() => child.kill("SIGKILL"), 5000);
      }, timeoutMs);
    }

    if (input !== undefined) {
      child.stdin?.write(input);
    }
    child.stdin?.end();
  });
}

/** Run a shell command string (sh -c), capturing output. */
export function sh(command: string, opts: SpawnOptions & { timeoutMs?: number } = {}): Promise<RunResult> {
  return run("/bin/sh", ["-c", command], opts);
}

export function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

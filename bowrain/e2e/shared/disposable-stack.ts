/**
 * The e2e suites delete every project in the workspace they use. They run only
 * against a server that reports itself as a disposable test stack on
 * /api/v1/health, which a server does only when started with
 * BOWRAIN_DISPOSABLE_STACK=1 (and the development flag beside it). CI's stack
 * (bowrain/e2e/server/compose.yaml) sets it; a local stack started with
 * `BOWRAIN_DISPOSABLE_STACK=1 make -C bowrain stack-up-e2e` does too.
 *
 * Every suite helper calls requireDisposableStack before its first request, so
 * a base URL or a token for any other server stops the run before it changes
 * anything.
 */

const checked = new Map<string, Promise<void>>();

export function requireDisposableStack(baseUrl: string): Promise<void> {
  const base = baseUrl.replace(/\/+$/, "");
  let pending = checked.get(base);
  if (!pending) {
    pending = check(base);
    checked.set(base, pending);
  }
  return pending;
}

async function check(base: string): Promise<void> {
  const url = `${base}/api/v1/health`;
  let body: unknown;
  try {
    const resp = await fetch(url);
    body = resp.ok ? await resp.json() : undefined;
  } catch {
    body = undefined;
  }
  if ((body as { disposable?: unknown } | undefined)?.disposable === true) return;
  throw new Error(
    `Refusing to run against ${base}: its ${url} does not report a disposable test stack. ` +
      "These suites delete every project in the workspace they use. " +
      "Start the stack with BOWRAIN_DISPOSABLE_STACK=1 (CI's bowrain/e2e/server/compose.yaml sets it), " +
      "or point BOWRAIN_URL / BOWRAIN_SERVER_URL at a stack that was.",
  );
}

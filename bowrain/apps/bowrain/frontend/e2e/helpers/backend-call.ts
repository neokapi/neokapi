/**
 * Unified helper to call Wails backend methods from test code.
 * Auto-detects mock mode vs server mode (headless binary).
 *
 * Mock mode:  delegates to window.__wailsMockByName (injected by setupLocalApp)
 * Server mode: calls the Wails runtime HTTP endpoint using the binding protocol
 */
import type { Page } from "@playwright/test";

/**
 * Wails binding method IDs from the generated app.js bindings.
 * These must match the IDs in mock-backend.ts and the generated bindings.
 */
const METHOD_IDS: Record<string, number> = {
  AddConcept: 719452361,
  AddItems: 1434721329,
  AddTMEntry: 877455352,
  CreateProject: 2130069333,
  GetItemBlocks: 2088446217,
  ListProjects: 1255557302,
  ListProjectFiles: 882190332,
  PseudoTranslateItem: 3367128751,
};

/**
 * Call a Wails backend method by name. Works in both mock and server mode.
 *
 * @param page  Playwright page instance
 * @param method  Method name (e.g., "ListProjects", "AddItems")
 * @param args  Arguments to pass to the method
 * @returns The method result (parsed from JSON in server mode)
 */
export async function callBackend(page: Page, method: string, ...args: any[]) {
  const methodId = METHOD_IDS[method];
  if (methodId === undefined) {
    throw new Error(`callBackend: unknown method "${method}". Add it to METHOD_IDS.`);
  }

  return page.evaluate(
    async ({ method, methodId, args }) => {
      // Mock mode: __wailsMockByName is injected by setupLocalApp
      if ((window as any).__wailsMockByName) {
        return (window as any).__wailsMockByName[method](...args);
      }

      // Server mode: call via Wails runtime HTTP protocol
      // The Wails v3 runtime routes binding calls through POST /wails/runtime
      // objectNames.Call = 0, CallBinding = 0
      const callId = Math.random().toString(36).slice(2);
      const resp = await fetch("/wails/runtime", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "x-wails-client-id": "e2e-test",
        },
        body: JSON.stringify({
          object: 0,
          method: 0,
          args: { "call-id": callId, methodID: methodId, args },
        }),
      });

      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`callBackend(${method}) failed: ${resp.status} ${text}`);
      }

      return resp.json();
    },
    { method, methodId, args },
  );
}

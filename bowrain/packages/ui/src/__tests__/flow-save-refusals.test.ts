import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";
import { parseAppError, type AppError } from "../errors/parseAppError";
import { NetworkError } from "../errors/NetworkError";
import type { FlowDefinitionInfo } from "../types/api";

// Every refusal a save can meet, as the server words it, reaching the web
// app and the desktop app in their own shapes. Both render through
// parseAppError, so the person reads the same thing on both: what to do,
// never a URL or a status code.
const refusals: Array<[number, string]> = [
  [400, "edge e1 references a node that does not exist"],
  [403, "cannot modify a built-in flow"],
  [403, "insufficient permissions"],
  [404, "this flow was deleted after it was read"],
  [409, "a flow with this id exists; save it with a PUT to change it"],
  [428, "this version of the app cannot save flows; update it, then save again"],
];

const mine: FlowDefinitionInfo = {
  id: "flow-1",
  name: "Mine",
  source: "project",
  nodes: [],
  edges: [],
  revision: "r1",
};

afterEach(() => vi.unstubAllGlobals());

/** The web app's refusal: what the REST adapter throws for the response. */
async function onTheWeb(status: number, sentence: string): Promise<unknown> {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: false,
      status,
      json: async () => ({ error: sentence }),
      text: async () => JSON.stringify({ error: sentence }),
      headers: { get: () => null },
    }),
  );
  return new RestApiAdapter("http://server")
    .updateFlowDefinition("acme", "p-1", "flow-1", mine)
    .catch((e: unknown) => e);
}

/**
 * The desktop app's refusal: the rejection the Wails runtime delivers for the
 * Go error, its message the error's text and its cause the error's JSON
 * (apiclient.RefusalJSON, pinned by TestRefusalJSONCarriesTheServersAnswer).
 */
function onTheDesktop(message: string, cause: unknown): unknown {
  const err = new Error(message);
  err.name = "RuntimeError";
  (err as Error & { cause: unknown }).cause = cause;
  return err;
}

function shown(e: unknown): Pick<AppError, "title" | "detail" | "hint"> {
  const { title, detail, hint } = parseAppError(e);
  return { title, detail, hint };
}

function expectActionable(e: unknown, status?: number) {
  const { title, detail, hint } = shown(e);
  const text = [title, detail, hint].filter(Boolean).join(" ");
  expect(title).not.toBe("Something went wrong");
  expect(text).not.toMatch(/https?:\/\//);
  if (status !== undefined) expect(text).not.toContain(String(status));
}

describe("a refused save", () => {
  for (const [status, sentence] of refusals) {
    it(`${status} "${sentence}" reads the same on the web and the desktop`, async () => {
      const web = await onTheWeb(status, sentence);
      const desktop = onTheDesktop(sentence, { status, body: { error: sentence } });

      expect(shown(desktop)).toEqual(shown(web));
      expectActionable(web, status);
      const text = [shown(web).title, shown(web).detail].filter(Boolean).join(" ");
      expect(text.toLowerCase()).toContain(sentence.toLowerCase());
    });
  }

  it("a response without a body of its own reads as its status, the same on both", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 502,
        json: async () => {
          throw new Error("not JSON");
        },
        text: async () => "<html>Bad gateway</html>",
        headers: { get: () => null },
      }),
    );
    const web = await new RestApiAdapter("http://server")
      .updateFlowDefinition("acme", "p-1", "flow-1", mine)
      .catch((e: unknown) => e);
    const desktop = onTheDesktop("the server could not save the flow (bad gateway)", {
      status: 502,
      body: "<html>Bad gateway</html>",
    });

    expect(shown(desktop)).toEqual(shown(web));
    expectActionable(web, 502);
  });

  it("a save that never reached the server says so, the same on both", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    const web = await new RestApiAdapter("http://server")
      .updateFlowDefinition("acme", "p-1", "flow-1", mine)
      .catch((e: unknown) => e);
    const desktop = onTheDesktop(
      'could not reach the server: Put "https://acme.test/api/v1/w/p/flows/flow-1": dial tcp: connection refused',
      { network: true },
    );

    expect(web).toBeInstanceOf(NetworkError);
    expect(shown(desktop)).toEqual(shown(web));
    expect(shown(web).title).toBe("The server could not be reached");
    expectActionable(desktop);
  });

  it("a request the caller stopped is not reported as a network failure", async () => {
    const abort = new DOMException("The operation was aborted.", "AbortError");
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(abort));
    const err = await new RestApiAdapter("http://server")
      .updateFlowDefinition("acme", "p-1", "flow-1", mine)
      .catch((e: unknown) => e);
    expect(err).toBe(abort);
  });
});

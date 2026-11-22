import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";
import { parseAppError } from "../errors/parseAppError";
import type { FlowDefinitionInfo } from "../types/api";

// Every refusal a flow save can meet, as the server words it. The editor shows
// each one under "Could not save the flow"; the person reads what to do, never
// a URL or a status code.
const refusals: Array<[number, string]> = [
  [400, "edge e1 references a node that does not exist"],
  [403, "cannot modify a built-in flow"],
  [404, "this flow was deleted after it was read"],
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

describe("a refused flow save", () => {
  for (const [status, sentence] of refusals) {
    it(`${status} reads as the server's sentence`, async () => {
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
      const err = await new RestApiAdapter("http://server")
        .updateFlowDefinition("acme", "p-1", "flow-1", mine)
        .catch((e: unknown) => e);
      const shown = parseAppError(err);
      const text = [shown.title, shown.detail].filter(Boolean).join(" ");
      expect(text.toLowerCase()).toContain(sentence);
      expect(text).not.toMatch(/https?:\/\//);
      expect(text).not.toContain(String(status));
    });
  }
});

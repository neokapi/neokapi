import { test, expect } from "../fixtures/test";

test.describe("AI Features", () => {
  test("readiness reports AI component status", async ({ readiness }) => {
    const ai = readiness.components.ai;
    expect(ai).toBeTruthy();

    if (ai.status === "up" && ai.providers) {
      // At least one AI provider is configured.
      const configured = ai.providers.filter((p) => p.configured);
      expect(configured.length).toBeGreaterThan(0);

      for (const provider of configured) {
        expect(provider.name).toBeTruthy();
        // Provider has a model configured.
        expect(provider.model).toBeTruthy();
      }
    } else {
      // AI is not configured — skip dependent tests.
      test.info().annotations.push({
        type: "skip",
        description: "AI providers not configured on this server",
      });
    }
  });
});

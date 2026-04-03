import { test, expect } from "../fixtures/test";

const SAMPLE_JSON = JSON.stringify({
  title: "Stream Test",
  description: "Testing stream management",
});

test.describe("Stream Management", () => {
  let wsSlug: string;
  let projectId: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace(
      "E2E Streams",
      `e2e-streams-${Date.now().toString(36)}`,
    );
    wsSlug = ws.slug;
    const project = await api.createProject(wsSlug, "Stream Project", "en", ["fr", "de"]);
    projectId = project.id;
    await api.uploadFile(wsSlug, projectId, "app.json", SAMPLE_JSON);
  });

  test("list streams (main exists by default)", async ({ api }) => {
    let streams: Array<{ name: string }>;
    try {
      streams = await api.listStreams(wsSlug, projectId);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Streams feature not available on this server");
        return;
      }
      throw err;
    }

    expect(streams.length).toBeGreaterThanOrEqual(1);
    const main = streams.find((s) => s.name === "main");
    expect(main).toBeTruthy();
  });

  test("create a new stream", async ({ api }) => {
    let stream: { name: string };
    try {
      stream = await api.createStream(wsSlug, projectId, {
        name: "feature-e2e-test",
        description: "E2E test stream",
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Streams feature not available on this server");
        return;
      }
      throw err;
    }

    expect(stream.name).toBe("feature-e2e-test");
  });

  test("get stream details", async ({ api }) => {
    let streams: Array<{ name: string }>;
    try {
      streams = await api.listStreams(wsSlug, projectId);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Streams feature not available on this server");
        return;
      }
      throw err;
    }

    const target = streams.find((s) => s.name === "feature-e2e-test");
    if (!target) {
      test.skip(true, "Stream was not created in previous test");
      return;
    }

    const detail = await api.getStream(wsSlug, projectId, target.name);
    expect(detail.name).toBe("feature-e2e-test");
  });

  test("delete stream", async ({ api }) => {
    let stream: { name: string };
    try {
      stream = await api.createStream(wsSlug, projectId, {
        name: "feature-to-delete",
        description: "Will be deleted",
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Streams feature not available on this server");
        return;
      }
      throw err;
    }

    await api.deleteStream(wsSlug, projectId, stream.name);

    const streams = await api.listStreams(wsSlug, projectId);
    const found = streams.find((s) => s.name === stream.name);
    expect(found).toBeUndefined();
  });
});

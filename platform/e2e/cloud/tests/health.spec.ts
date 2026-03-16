import { test, expect } from "../fixtures/test";

test.describe("Health & Readiness", () => {
  test("health endpoint returns ok", async ({ api }) => {
    const health = await api.health();
    expect(health.status).toBe("ok");
  });

  test("readiness endpoint reports component status", async ({ readiness }) => {
    expect(readiness.status).toMatch(/ready|degraded|unhealthy/);
    expect(readiness.version).toBeTruthy();
    expect(readiness.components.database).toBeTruthy();
    expect(readiness.components.database.status).toBe("up");
    expect(readiness.components.queue).toBeTruthy();
    expect(readiness.components.session_store).toBeTruthy();
  });

  test("info endpoint returns version", async ({ api }) => {
    const info = await api.info();
    expect(info.version).toBeTruthy();
    expect(info.commit).toBeTruthy();
  });

  test("formats endpoint lists available formats", async ({ api }) => {
    const formats = await api.listFormats();
    expect(formats.length).toBeGreaterThan(0);
  });

  test("tools endpoint lists available tools", async ({ api }) => {
    const tools = await api.listTools();
    expect(tools.length).toBeGreaterThan(0);
  });
});

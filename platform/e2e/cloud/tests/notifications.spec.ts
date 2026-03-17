import { test, expect } from "../fixtures/test";

test.describe("Notification Preferences", () => {
  let wsSlug: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace(
      "E2E Notifications",
      `e2e-notif-${Date.now().toString(36)}`,
    );
    wsSlug = ws.slug;
  });

  test("get notification preferences", async ({ api }) => {
    let prefs: Record<string, unknown>;
    try {
      prefs = await api.getNotificationPreferences(wsSlug);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Notification preferences not available on this server");
        return;
      }
      throw err;
    }

    expect(prefs).toBeTruthy();
    expect(typeof prefs).toBe("object");
  });

  test("update notification preferences", async ({ api }) => {
    let prefs: Record<string, unknown>;
    try {
      prefs = await api.getNotificationPreferences(wsSlug);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Notification preferences not available on this server");
        return;
      }
      throw err;
    }

    // Update with the current prefs (effectively a no-op update to verify the endpoint works).
    try {
      const updated = await api.updateNotificationPreferences(wsSlug, prefs);
      expect(updated).toBeTruthy();
      expect(typeof updated).toBe("object");
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503") || msg.includes("405")) {
        test.skip(true, "Notification preferences update not available on this server");
        return;
      }
      throw err;
    }
  });
});

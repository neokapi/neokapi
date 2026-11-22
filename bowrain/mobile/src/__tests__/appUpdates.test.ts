import { CHECK_INTERVAL_MS, createUpdateController, type UpdatesApi } from "../lib/appUpdates";

function fakeUpdates(available: boolean) {
  const calls: string[] = [];
  const api: UpdatesApi = {
    isEnabled: true,
    checkForUpdateAsync: async () => {
      calls.push("check");
      return { isAvailable: available };
    },
    fetchUpdateAsync: async () => {
      calls.push("fetch");
      return { isNew: available };
    },
    reloadAsync: async () => {
      calls.push("reload");
    },
  };
  return { api, calls };
}

describe("the update controller", () => {
  it("downloads on launch and reloads at the next foreground", async () => {
    const { api, calls } = fakeUpdates(true);
    const saved: string[] = [];
    const c = createUpdateController({
      updates: api,
      isBusy: () => false,
      beforeReload: async () => {
        saved.push("saved");
      },
    });
    await c.check(0);
    expect(c.ready()).toBe(true);
    expect(calls).toEqual(["check", "fetch"]);
    await c.foreground(1);
    expect(saved).toEqual(["saved"]);
    expect(calls).toEqual(["check", "fetch", "reload"]);
  });

  it("waits while the person is busy and stays ready for the banner", async () => {
    const { api, calls } = fakeUpdates(true);
    let busy = true;
    const c = createUpdateController({
      updates: api,
      isBusy: () => busy,
      beforeReload: async () => {},
    });
    let notified = 0;
    c.subscribe(() => notified++);
    await c.check(0);
    expect(notified).toBe(1);
    await c.foreground(1);
    expect(calls).not.toContain("reload");
    busy = false;
    await c.foreground(2);
    expect(calls).toContain("reload");
  });

  it("checks again on foreground only after the interval", async () => {
    const { api, calls } = fakeUpdates(false);
    const c = createUpdateController({
      updates: api,
      isBusy: () => false,
      beforeReload: async () => {},
    });
    await c.check(0);
    await c.foreground(CHECK_INTERVAL_MS - 1);
    expect(calls).toEqual(["check"]);
    await c.foreground(CHECK_INTERVAL_MS);
    expect(calls).toEqual(["check", "check"]);
    expect(c.ready()).toBe(false);
  });

  it("does nothing when updates are off or the server cannot be reached", async () => {
    const off = createUpdateController({
      updates: null,
      isBusy: () => false,
      beforeReload: async () => {},
    });
    await off.check(0);
    await off.apply();
    expect(off.ready()).toBe(false);

    const failing = createUpdateController({
      updates: {
        isEnabled: true,
        checkForUpdateAsync: async () => {
          throw new Error("offline");
        },
        fetchUpdateAsync: async () => ({ isNew: false }),
        reloadAsync: async () => {},
      },
      isBusy: () => false,
      beforeReload: async () => {},
    });
    await expect(failing.check(0)).resolves.toBeUndefined();
    expect(failing.ready()).toBe(false);
  });
});

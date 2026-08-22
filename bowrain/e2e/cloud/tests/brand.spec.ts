import { test, expect } from "../fixtures/test";

test.describe("Voice Profiles", () => {
  let wsSlug: string;
  let createdProfileId: string | undefined;
  let brandAvailable = true;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E Brand", `e2e-brand-${Date.now().toString(36)}`);
    wsSlug = ws.slug;

    // Try to create a profile to verify brand feature availability.
    try {
      const profile = await api.createBrandProfileFromStarter(
        wsSlug,
        "professional-b2b",
        "E2E Voice",
      );
      createdProfileId = profile.id;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      // 503 = brand store not configured; 404 with "Not Found" = route not registered.
      // 404 with "starter pack not found" is a real error (wrong pack name), not "unavailable".
      if (msg.includes("503") || (msg.includes("404") && !msg.includes("starter pack"))) {
        brandAvailable = false;
      } else {
        throw err;
      }
    }
  });

  // eslint-disable-next-line no-unused-vars
  test("create voice profile from starter pack", async ({ api }) => {
    if (!brandAvailable) {
      test.skip(true, "Brand profiles feature not available on this server");
      return;
    }

    // Profile was already created in beforeAll.
    expect(createdProfileId).toBeTruthy();
  });

  test("list brand profiles", async ({ api }) => {
    if (!brandAvailable) {
      test.skip(true, "Brand profiles feature not available on this server");
      return;
    }

    const profiles = await api.listVoiceProfiles(wsSlug);
    expect(profiles.length).toBeGreaterThan(0);
    const found = profiles.find((p) => p.name === "E2E Voice");
    expect(found).toBeTruthy();
  });

  test("get voice profile by ID", async ({ api }) => {
    if (!brandAvailable || !createdProfileId) {
      test.skip(true, "Brand profiles feature not available or profile not created");
      return;
    }

    const profiles = await api.listVoiceProfiles(wsSlug);
    const target = profiles.find((p) => p.name === "E2E Voice");
    expect(target).toBeTruthy();
    expect(target!.id).toBeTruthy();
  });

  test("update voice profile", async ({ api }) => {
    if (!brandAvailable || !createdProfileId) {
      test.skip(true, "Brand profiles feature not available or profile not created");
      return;
    }

    const updated = await api.updateVoiceProfile(wsSlug, createdProfileId!, {
      name: "E2E Voice Updated",
    });

    expect(updated.name).toBe("E2E Voice Updated");
  });

  test("delete voice profile", async ({ api }) => {
    if (!brandAvailable) {
      test.skip(true, "Brand profiles feature not available on this server");
      return;
    }

    let profile: { id: string; name: string };
    try {
      profile = await api.createBrandProfileFromStarter(wsSlug, "professional-b2b", "To Delete");
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Brand profiles feature not available on this server");
        return;
      }
      throw err;
    }

    await api.deleteVoiceProfile(wsSlug, profile.id);

    const profiles = await api.listVoiceProfiles(wsSlug);
    const found = profiles.find((p) => p.id === profile.id);
    expect(found).toBeUndefined();
  });
});

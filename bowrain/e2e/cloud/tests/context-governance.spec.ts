import { test, expect } from "../fixtures/test";
import type { BowrainAPI, ChannelAliasProposal, SyncContextEntry } from "../helpers/api-client";

/**
 * The workspace's governance surfaces, against the real stack.
 *
 * Both scenarios need state only a real push produces, so both push the context
 * content type the way a recipe does: the collections it declares, their
 * coordinates, and the voice governing each. Nothing is faked into the store.
 */

const PRODUCT = "acme";

/** A voice carrying enough vocabulary that a card can count rules at the point. */
function voiceProfile(name: string): string {
  return Buffer.from(
    JSON.stringify({
      name,
      description: "How this point sounds.",
      vocabulary: {
        preferred_terms: [{ term: "sign in", replacement: "log in" }],
        forbidden_terms: [{ term: "utilize", replacement: "use", severity: "major" }],
      },
    }),
  ).toString("base64");
}

function collection(name: string, channel: string, voice?: string): SyncContextEntry {
  return {
    name,
    coordinates: { product: PRODUCT, channel },
    channel,
    owner: "recipe",
    content_hash: `${PRODUCT}:${channel}:${name}`,
    ...(voice ? { voice_profile: voice, voice_profile_json: voiceProfile(voice) } : {}),
  };
}

/** Waits for the push worker to reconcile, then for the graph pass behind it. */
async function waitForCollection(
  api: BowrainAPI,
  wsSlug: string,
  collectionName: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { profiles } = await api.listContextProfiles(wsSlug);
        return profiles.some((p) => p.collections.some((c) => c.name === collectionName));
      },
      { timeout: 60_000, intervals: [1_000] },
    )
    .toBe(true);
}

async function waitForProposal(
  api: BowrainAPI,
  wsSlug: string,
  proposed: string,
): Promise<ChannelAliasProposal> {
  let found: ChannelAliasProposal | undefined;
  await expect
    .poll(
      async () => {
        const { proposals } = await api.listChannelProposals(wsSlug);
        found = proposals.find((p) => p.proposed_channel === proposed);
        return !!found;
      },
      { timeout: 60_000, intervals: [1_000] },
    )
    .toBe(true);
  return found!;
}

test.describe("Context governance", () => {
  let wsSlug: string;
  let heldProject: string;
  let arrivingProject: string;

  test.beforeAll(async ({ api }) => {
    const suffix = Date.now().toString(36);
    const ws = await api.getOrCreateWorkspace("E2E Context", `e2e-context-${suffix}`);
    wsSlug = ws.slug;

    const held = await api.createWorkspaceProject(wsSlug, "Support Site", "en", ["nb"]);
    const arriving = await api.createWorkspaceProject(wsSlug, "Help App", "en", ["nb"]);
    heldProject = held.id;
    arrivingProject = arriving.id;

    // The workspace already holds `help-centre` under the acme product.
    await api.pushContext(wsSlug, heldProject, "main", [
      collection("support-articles", "help-centre", "Acme support voice"),
    ]);
    await waitForCollection(api, wsSlug, "support-articles");

    // A second project arrives spelling the same surface `help`.
    await api.pushContext(wsSlug, arrivingProject, "main", [
      collection("in-app-help", "help", "Acme support voice"),
    ]);
    await waitForCollection(api, wsSlug, "in-app-help");
  });

  test("a profile card carries the point, its voice, and its check standing", async ({
    api,
    authenticatedPage: page,
  }) => {
    const { profiles } = await api.listContextProfiles(wsSlug);
    const point = profiles.find((p) => p.channel === "help-centre");
    expect(point, "the push declared a point on the channel axis").toBeTruthy();
    expect(point!.voice?.name).toBe("Acme support voice");

    await page.goto(`/${wsSlug}/context/profiles`);
    await expect(page.getByRole("heading", { name: "Profiles" })).toBeVisible({ timeout: 30_000 });

    // The workspace's own point is labelled Brand, and each declared point
    // carries its conventional name.
    await expect(page.getByRole("heading", { name: "Brand", exact: true })).toBeVisible();
    const card = page.locator("[data-slot='card']", { hasText: point!.label }).first();
    await expect(card).toBeVisible();
    await expect(card.getByText("Acme support voice")).toBeVisible();
    // Nothing here has been checked, so the card says that rather than a zero.
    await expect(card.getByText("Not checked yet")).toBeVisible();

    // The detail carries the same standing as a section, and says what scopes it.
    await card.click();
    await expect(page.getByRole("heading", { name: "Standing" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(/Nothing here has been checked/)).toBeVisible();
  });

  test("a raised pair is judged in the hub, and the next push leaves it judged", async ({
    api,
    authenticatedPage: page,
  }) => {
    const raised = await waitForProposal(api, wsSlug, "help");
    expect(raised.existing_channel).toBe("help-centre");
    expect(raised.profile).toBe(PRODUCT);
    expect(raised.status).toBe("proposed");
    expect(raised.evidence).toBeTruthy();

    await page.goto(`/${wsSlug}/context/profiles`);
    const panel = page.getByTestId("channel-proposals");
    await expect(panel).toBeVisible({ timeout: 30_000 });
    await expect(panel.getByText("help", { exact: true })).toBeVisible();
    await expect(panel.getByText("help-centre", { exact: true })).toBeVisible();

    await panel.getByRole("button", { name: /one channel/i }).click();
    // The verdicts give way to the judgement they recorded.
    await expect(panel.getByRole("button", { name: /one channel/i })).toHaveCount(0, {
      timeout: 15_000,
    });
    await expect(panel.getByText("One channel", { exact: true })).toBeVisible();

    // The judgement is the server's, not the page's.
    await expect
      .poll(
        async () => {
          const { proposals } = await api.listChannelProposals(wsSlug);
          return proposals.find((p) => p.proposed_channel === "help")?.status;
        },
        { timeout: 15_000, intervals: [500] },
      )
      .toBe("accepted");

    // The same fragmentation is observed again on the next push. A re-sighting
    // refreshes where it was seen; it must not reopen the judgement.
    await api.pushContext(wsSlug, arrivingProject, "main", [
      collection("in-app-help", "help", "Acme support voice"),
      collection("in-app-tips", "help", "Acme support voice"),
    ]);
    await waitForCollection(api, wsSlug, "in-app-tips");

    const { proposals: open } = await api.listChannelProposals(wsSlug, "proposed");
    expect(open.find((p) => p.proposed_channel === "help")).toBeUndefined();

    const { proposals: all } = await api.listChannelProposals(wsSlug);
    expect(all.find((p) => p.proposed_channel === "help")?.status).toBe("accepted");

    // And neither project's slug moved: the workspace judges equivalence,
    // never resolution.
    const { profiles } = await api.listContextProfiles(wsSlug);
    expect(profiles.some((p) => p.channel === "help")).toBe(true);
    expect(profiles.some((p) => p.channel === "help-centre")).toBe(true);
  });
});

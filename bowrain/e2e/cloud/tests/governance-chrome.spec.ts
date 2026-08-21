import { test, expect } from "../fixtures/test";
import type { BowrainAPI, ChangeSetImpactInfo } from "../helpers/api-client";

/**
 * The governance journey against the real stack: propose → reach → trial →
 * approve → merge, on content a real upload produced.
 *
 * Nothing here is seeded past the API. A file is uploaded and parsed into real
 * blocks, a concept is curated the ordinary way, and the ban travels the only
 * road a governed change has — a reviewed change-set. What Reach and Trial say
 * is then read back from the same endpoints the panels read.
 */

const TERM = "utilise";
const SUCCESSOR = "use";

/** Two sentences carrying the term, and one that does not. */
const CONTENT = JSON.stringify(
  {
    intro: `You can ${TERM} the API to fetch a quote.`,
    pricing: `Teams that ${TERM} the batch endpoint pay less.`,
    footer: "All prices exclude tax.",
  },
  null,
  2,
);

/**
 * The blast radius is walked when it is asked for, but the blocks it walks
 * arrive through the upload pipeline, so the first read can land before the
 * parse has stored anything. Poll until the walk has content to see.
 */
async function blastRadiusOverContent(
  api: BowrainAPI,
  wsSlug: string,
  changesetId: string,
): Promise<ChangeSetImpactInfo> {
  let impact: ChangeSetImpactInfo | undefined;
  await expect
    .poll(
      async () => {
        impact = await api.changesetBlastRadius(wsSlug, changesetId, true);
        return impact.total_blocks;
      },
      { timeout: 60_000, intervals: [1_000] },
    )
    .toBeGreaterThan(0);
  return impact!;
}

test.describe("Governance chrome", () => {
  let wsSlug: string;
  let projectId: string;
  let conceptId: string;
  let successorId: string;

  test.beforeAll(async ({ api }) => {
    const suffix = Date.now().toString(36);
    const ws = await api.getOrCreateWorkspace("E2E Chrome", `e2e-chrome-${suffix}`);
    wsSlug = ws.slug;

    const project = await api.createProject(wsSlug, "Pricing Site", "en", ["nb"]);
    projectId = project.id;
    // Real content through the real parse — the walk has nothing to see
    // otherwise, and a blast radius over nothing proves nothing.
    await api.uploadFile(wsSlug, projectId, "pricing.json", CONTENT);

    // Two concepts: the one to ban, and the one it should give way to. The
    // successor is what turns the ban from an annotation into an instruction to
    // rewrite the source, which is the distinction Reach exists to price.
    const banned = await api.addConcept(wsSlug, {
      domain: "product",
      definition: "A word we no longer write.",
      terms: [{ text: TERM, locale: "en", status: "admitted" }],
    });
    conceptId = banned.id;
    const successor = await api.addConcept(wsSlug, {
      domain: "product",
      definition: "What we write instead.",
      terms: [{ text: SUCCESSOR, locale: "en", status: "admitted" }],
    });
    successorId = successor.id;
  });

  test("a term ban is proposed, priced, tried, approved, and merged", async ({ api }) => {
    // ── Propose ──────────────────────────────────────────────────────────
    const cs = await api.createChangeset(wsSlug, `Retire "${TERM}"`, "It reads as jargon.");
    expect(cs.status).toBe("draft");

    // The ban itself, plus the guidance that names its successor.
    await api.addChangesetOp(wsSlug, cs.id, "term.status", {
      concept_id: conceptId,
      locale: "en",
      text: TERM,
      from: "admitted",
      to: "forbidden",
    });
    await api.addChangesetOp(wsSlug, cs.id, "relation.add", {
      relation: {
        id: `r-${cs.id}`,
        source_id: conceptId,
        target_id: successorId,
        relation_type: "USE_INSTEAD",
      },
    });

    // ── Reach ────────────────────────────────────────────────────────────
    const impact = await blastRadiusOverContent(api, wsSlug, cs.id);
    expect(impact.affected_blocks, "the two blocks carrying the term").toBe(2);
    expect(impact.new_violations).toBe(2);

    const reach = impact.reach;
    expect(reach, "the blast radius carries the cost split").toBeTruthy();
    expect(
      reach!.transform.blocks,
      "a ban that names a successor is an instruction to rewrite the source",
    ).toBe(2);
    expect(reach!.annotate.blocks, "and nothing here is merely flagged").toBe(0);
    expect(reach!.transform.blocks + reach!.annotate.blocks).toBe(impact.affected_blocks);
    expect(reach!.transform.projects).toBe(1);
    expect(reach!.transform_projects.map((p) => p.project_id)).toEqual([projectId]);
    // Nothing has been translated yet, so nothing is invalidated — the honest
    // answer, and the one that changes the moment a target exists.
    expect(reach!.transform.targets).toBe(0);
    expect(reach!.transform.collections).toBeGreaterThan(0);

    // ── Trial ────────────────────────────────────────────────────────────
    // The findings diff names the rule, not just a count. It answers before a
    // pilot exists, so a reviewer can look before binding anything.
    const trial = await api.trialFindings(wsSlug, cs.id, projectId, "main");
    expect(trial.stream).toBe("main");
    expect(trial.changed_blocks).toBe(2);
    expect(trial.raised_total).toBe(2);
    expect(trial.cleared_total).toBe(0);
    expect(trial.raised.map((f) => f.rule)).toEqual([TERM, TERM]);
    expect(trial.raised[0].kind).toBe("term");
    expect(trial.raised[0].replacement).toBe(SUCCESSOR);
    expect(trial.terms_computed, "the terms half is applied for the report").toBe(true);
    expect(trial.voice_bound, "a terms-only draft binds no candidate profile").toBeFalsy();

    // Binding a pilot must not change what the workspace's own checks see: the
    // shadow belongs to its stream. The trial on main reads the same after.
    await api.startPilot(wsSlug, cs.id, projectId, "main");
    const afterPilot = await api.trialFindings(wsSlug, cs.id, projectId, "main");
    expect(afterPilot.raised_total).toBe(trial.raised_total);

    // ── Approve ──────────────────────────────────────────────────────────
    const submitted = await api.submitChangeset(wsSlug, cs.id);
    expect(submitted.status).toBe("in_review");

    const detail = await api.getChangeset(wsSlug, cs.id);
    expect(detail.governed, "a forbidden transition is governed").toBe(true);
    expect(
      detail.solo_review,
      "the only member holding manage_voice is the author, so a verdict is admitted on the solo-owner basis",
    ).toBe(true);

    const approved = await api.approveChangeset(wsSlug, cs.id, "Reads better.");
    expect(approved.status).toBe("approved");

    const merged = await api.mergeChangeset(wsSlug, cs.id);
    expect(merged.conflicts ?? []).toHaveLength(0);

    // ── The ledger ───────────────────────────────────────────────────────
    const final = await api.getChangeset(wsSlug, cs.id);
    expect(final.status).toBe("merged");
    expect(final.merged_at).toBeTruthy();
    expect(final.reviews?.[0]?.verdict).toBe("approve");
    expect(final.reviews?.[0]?.basis).toBe("solo_owner");

    // And the decision reached the activity feed, which is the readable half of
    // the ledger the audit chain also carries.
    await expect
      .poll(
        async () => {
          const activities = await api.listActivities(wsSlug);
          return activities.some((a) => (a as { entity_id?: string }).entity_id === cs.id);
        },
        { timeout: 30_000, intervals: [1_000] },
      )
      .toBe(true);
  });

  test("the change reaches the page, and the panels say what the endpoints say", async ({
    api,
    authenticatedPage: page,
  }) => {
    const cs = await api.createChangeset(wsSlug, `Soften "${TERM}" again`, "A second proposal.");
    await api.addChangesetOp(wsSlug, cs.id, "term.status", {
      concept_id: conceptId,
      locale: "en",
      text: TERM,
      from: "forbidden",
      to: "admitted",
    });
    await blastRadiusOverContent(api, wsSlug, cs.id);

    await page.goto(`/${wsSlug}/context/changes/${cs.id}`);
    await expect(page.getByRole("heading", { name: "Blast radius" })).toBeVisible({
      timeout: 30_000,
    });

    // Reach renders under the hero and names the act, not only the number.
    await expect(page.getByRole("heading", { name: "What this asks for" })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText(/Re-check fans out|Source gets rewritten/)).toBeVisible();

    // The breakdown offers the collection axis the reach panel counts on.
    await expect(page.getByRole("tab", { name: "By collection" })).toBeVisible();

    // A pilot's trial is fetched when asked for, never on load.
    await api.startPilot(wsSlug, cs.id, projectId, "main");
    await page.reload();
    const compare = page.getByRole("button", { name: "Compare findings" });
    await expect(compare).toBeVisible({ timeout: 30_000 });
    await compare.click();
    await expect(page.getByText(/Trial on/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/Terms are computed for this report/)).toBeVisible();
  });
});

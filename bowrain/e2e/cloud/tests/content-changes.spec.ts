import { test, expect } from "../fixtures/test";
import type { BowrainAPI, EditorBlock } from "../helpers/api-client";

/**
 * Content changes against the live server, signed in through Keycloak: a save,
 * a review decision and a note are change sets (kapi.change/v1) on the stream's
 * changes route. Each save and decision names the revision of the translation
 * it was made against, so one made against wording that has since changed is
 * refused with the wording as it stands, in the API and in the web editor.
 */

const STRINGS = JSON.stringify({
  greeting: "Hello, world!",
  farewell: "Goodbye!",
});

const ITEM = "strings.json";

async function projectWithStrings(
  api: BowrainAPI,
  wsSlug: string,
  name: string,
): Promise<{ projectId: string; blocks: EditorBlock[] }> {
  const project = await api.createProject(wsSlug, name, "en", ["fr"]);
  await api.uploadFile(wsSlug, project.id, ITEM, STRINGS);
  const blocks = await api.getBlocks(wsSlug, project.id, ITEM);
  expect(blocks.length).toBeGreaterThan(0);
  return { projectId: project.id, blocks };
}

const setFrench = (blockId: string, ifMatch: string, text: string) => ({
  op: "set_content",
  at: { doc: ITEM, block: blockId, edition: "fr" },
  if_match: ifMatch,
  text,
});

test.describe("Content changes", () => {
  let wsSlug: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E Changes", `e2e-chg-${Date.now()}`);
    wsSlug = ws.slug;
  });

  test("a save names the revision it read, and a stale one is refused", async ({ api }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes API");
    const block = blocks[0];
    expect(block.target_revisions?.fr).toBe("absent");

    const first = await api.applyChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, "absent", "Bonjour, le monde !")],
    });
    expect(first.status).toBe("applied");
    expect(first.record).toBeTruthy();
    const saved = first.ops[0].after;
    expect(saved).toMatch(/^r:/);

    // A second save made against the revision the first one replaced.
    const stale = await api.sendChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, "absent", "Salut, le monde !")],
    });
    expect(stale.status).toBe(409);
    expect(stale.result.status).toBe("refused");
    expect(stale.result.ops[0].error?.code).toBe("stale");
    expect(stale.result.ops[0].current).toEqual({ rev: saved, text: "Bonjour, le monde !" });

    // Made again on the revision the refusal named, it lands.
    const again = await api.applyChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, saved!, "Salut, le monde !")],
    });
    expect(again.status).toBe("applied");
    const [served] = (await api.getBlocks(wsSlug, projectId, ITEM)).filter(
      (b) => b.id === block.id,
    );
    expect(served.targets?.fr?.text).toBe("Salut, le monde !");
    expect(served.target_revisions?.fr).toBe(again.ops[0].after);
  });

  test("a decision binds to the wording the reviewer read", async ({ api }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes review");
    const block = blocks[0];
    const written = await api.applyChanges(wsSlug, projectId, {
      gate: "report",
      ops: [setFrench(block.id, "absent", "Bonjour, le monde !")],
    });
    const read = written.ops[0].after!;
    const rewritten = await api.applyChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, read, "Bonjour à tous !")],
    });
    const current = rewritten.ops[0].after!;

    const decide = (ifMatch: string) => ({
      ops: [
        {
          op: "decide",
          at: { doc: ITEM, block: block.id, edition: "fr" },
          if_match: ifMatch,
          outcome: "reject",
        },
      ],
    });
    const stale = await api.sendChanges(wsSlug, projectId, decide(read));
    expect(stale.status).toBe(409);
    expect(stale.result.ops[0].current?.text).toBe("Bonjour à tous !");

    const decided = await api.applyChanges(wsSlug, projectId, decide(current));
    expect(decided.status).toBe("applied");
    const [served] = (await api.getBlocks(wsSlug, projectId, ITEM)).filter(
      (b) => b.id === block.id,
    );
    expect(served.targets?.fr?.status).toBe("draft");
  });

  test("an agent's pre-review shows beside the translation it judged and decides nothing", async ({
    api,
  }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes pre-review");
    const block = blocks[0];
    const written = await api.applyChanges(wsSlug, projectId, {
      gate: "report",
      ops: [setFrench(block.id, "absent", "Bonjour, le monde !")],
    });
    const read = written.ops[0].after!;
    const adviseOp = {
      op: "decide" as const,
      at: { doc: ITEM, block: block.id, edition: "fr" },
      if_match: read,
      outcome: "advise" as const,
      score: 72,
      reasons: ["Reads as machine output."],
    };

    // The queue shows a pre-review as AI advice, so a person sends none.
    const personal = await api.sendChanges(wsSlug, projectId, { ops: [adviseOp] });
    expect(personal.status).toBe(403);
    expect(personal.result.ops[0].error?.code).toBe("not_permitted");

    // An agent sends it through the server MCP, acting for the same user.
    const advised = await api.callMcpTool<{ status: string }>(
      "apply_edits",
      { project_id: projectId, ops: [adviseOp] },
      "e2e-agent",
    );
    expect(advised.isError).toBe(false);
    expect(advised.result.status).toBe("applied");

    // The queue and the unit's context show it; the translation keeps its rung.
    const queue = await api.listPendingReview(wsSlug, projectId, ["fr"]);
    const entry = queue.entries.find((e) => e.block_id === block.id && e.locale === "fr");
    expect(entry?.pre_review).toEqual({
      score: 72,
      reviewer: "agent/e2e-agent",
      reasons: ["Reads as machine output."],
    });
    const context = await api.getReviewContext(wsSlug, projectId, block.id, "fr");
    expect(context.judgement.ai_score).toBe(72);
    expect(context.judgement.ai_findings).toEqual([{ message: "Reads as machine output." }]);
    const [served] = (await api.getBlocks(wsSlug, projectId, ITEM)).filter(
      (b) => b.id === block.id,
    );
    expect(served.targets?.fr?.status).not.toBe("established");

    // Once the wording changes, the advice judged other words.
    await api.applyChanges(wsSlug, projectId, {
      gate: "report",
      ops: [setFrench(block.id, read, "Bonjour à tous !")],
    });
    const after = await api.listPendingReview(wsSlug, projectId, ["fr"]);
    expect(after.entries.find((e) => e.block_id === block.id)?.pre_review).toBeUndefined();
    const contextAfter = await api.getReviewContext(wsSlug, projectId, block.id, "fr");
    expect(contextAfter.judgement.ai_score).toBeUndefined();
  });

  test("a note lands on the block with its author", async ({ api, auth }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes notes");
    const block = blocks[0];
    const res = await api.applyChanges(wsSlug, projectId, {
      ops: [
        {
          op: "annotate",
          at: { doc: ITEM, block: block.id },
          type: "note",
          value: { text: "Is this the hero line?" },
        },
      ],
    });
    const id = res.ops[0].id;
    expect(id).toBeTruthy();

    const notes = await api.listNotes(wsSlug, projectId, block.id);
    expect(notes).toEqual([
      expect.objectContaining({ id, text: "Is this the hero line?", author: auth.name }),
    ]);
  });

  test("a save the checks refuse lands only when the person saves it anyway", async ({
    api,
    authenticatedPage: page,
  }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes checks");
    const block = blocks[0];

    // A French word the workspace's terms rule out, through the reviewed path.
    // The concept answers for English into French (it has a source term and
    // a French rendering to use instead), so it governs French translations.
    const concept = await api.addConcept(wsSlug, {
      domain: "product",
      definition: "A small device.",
      terms: [
        { text: "gadget", locale: "en", status: "admitted" },
        { text: "machin", locale: "fr", status: "admitted" },
        { text: "bidule", locale: "fr", status: "admitted" },
      ],
    });
    const cs = await api.createChangeset(wsSlug, 'Retire "bidule"', "Too casual.");
    await api.addChangesetOp(wsSlug, cs.id, "term.status", {
      concept_id: concept.id,
      locale: "fr",
      text: "bidule",
      from: "admitted",
      to: "forbidden",
    });
    await api.submitChangeset(wsSlug, cs.id);
    await api.approveChangeset(wsSlug, cs.id, "Agreed.");
    await api.mergeChangeset(wsSlug, cs.id);

    // The default gate refuses the wording, with what the checks found.
    const refused = await api.sendChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, "absent", "Bonjour, bidule !")],
    });
    expect(refused.status).toBe(422);
    expect(refused.result.ops[0].error?.code).toBe("gate_failed");
    expect(refused.result.ops[0].findings?.some((f) => f.fails)).toBe(true);

    // In the editor the person sees the findings and saves anyway.
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/source`);
    await page.getByTestId(`open-file-${ITEM}`).click();
    await page.getByTestId("file-preview-translate").click();
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 30_000 });

    await page.getByTestId("edit-btn").click();
    const editable = page.getByTestId("unified-target-editor").locator('[contenteditable="true"]');
    await editable.click();
    await editable.pressSequentially("Bonjour, bidule !");
    await page.getByTestId("unified-save").click();

    const dialog = page.getByTestId("check-findings-dialog");
    await expect(dialog).toBeVisible();
    await expect(page.getByTestId("check-finding").first()).toContainText("terms.vocabulary");
    let served = (await api.getBlocks(wsSlug, projectId, ITEM)).find((b) => b.id === block.id);
    expect(served?.targets?.fr?.text ?? "").toBe("");

    const saved = page.waitForResponse(
      (r) =>
        r.url().endsWith("/streams/main/changes") &&
        r.request().method() === "POST" &&
        r.request().postDataJSON()?.gate === "report" &&
        r.ok(),
    );
    await page.getByTestId("findings-override").click();
    await saved;
    await expect(dialog).toBeHidden();
    served = (await api.getBlocks(wsSlug, projectId, ITEM)).find((b) => b.id === block.id);
    expect(served?.targets?.fr?.text).toBe("Bonjour, bidule !");
  });

  test("the editor shows a translation someone saved meanwhile before saving over it", async ({
    api,
    authenticatedPage: page,
  }) => {
    const { projectId, blocks } = await projectWithStrings(api, wsSlug, "Changes editor");
    const block = blocks[0];

    await page.goto(`/${wsSlug}/p/${projectId}/s/main/source`);
    await page.getByTestId(`open-file-${ITEM}`).click();
    await page.getByTestId("file-preview-translate").click();
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 30_000 });

    await page.getByTestId("edit-btn").click();
    const editable = page.getByTestId("unified-target-editor").locator('[contenteditable="true"]');
    await editable.click();
    await editable.pressSequentially("Bonjour, tout le monde !");

    // Someone else saves the same translation while the editor is open.
    await api.applyChanges(wsSlug, projectId, {
      ops: [setFrench(block.id, "absent", "Bonjour, le monde !")],
    });

    await page.getByTestId("unified-save").click();
    const dialog = page.getByTestId("stale-change-dialog");
    await expect(dialog).toBeVisible();
    await expect(page.getByTestId("stale-current")).toHaveText("Bonjour, le monde !");
    await expect(page.getByTestId("stale-mine")).toContainText("Bonjour, tout le monde !");

    // Saving over it applies the edit to the translation that stands.
    const saved = page.waitForResponse(
      (r) => r.url().endsWith("/streams/main/changes") && r.request().method() === "POST" && r.ok(),
    );
    await page.getByTestId("stale-reapply").click();
    await saved;
    await expect(dialog).toBeHidden();
    const [served] = (await api.getBlocks(wsSlug, projectId, ITEM)).filter(
      (b) => b.id === block.id,
    );
    expect(served.targets?.fr?.text).toBe("Bonjour, tout le monde !");
  });
});

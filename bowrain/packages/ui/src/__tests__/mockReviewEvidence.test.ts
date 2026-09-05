/**
 * The mock backend's two review calls have to agree with each other.
 *
 * The queue payload and the review context are separate calls over one
 * persisted block score on the server, so a story that seeds one and hardcodes
 * the other shows a contradiction: "Clears every bar" beside a 62-of-90 voice
 * score and a red major finding, which is what the ReviewSession story showed.
 * These read both calls for every seeded block and hold them to each other.
 */
import { describe, it, expect } from "vite-plus/test";

import { createMockAdapter } from "../stories/mock-adapter";
import {
  entryBlockers,
  entryVerdict,
  isBelowVoiceBar,
  type ReviewEntry,
} from "../components/review/reviewQueue";
import type { BlockInfo } from "../types/api";

function block(id: string, source: string, target: string): BlockInfo {
  return {
    id,
    source,
    source_coded: source,
    source_spans: [],
    targets: { "fr-FR": { text: target, status: "translated" } },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

const blocks = [
  block("b1", "Welcome to your dashboard", "Bienvenue sur votre tableau de bord"),
  block("b2", "Save changes", "Enregistrer les modifications"),
  block("b3", "Delete account", "Supprimer le compte"),
];

/** One block below its bar, one clearing it, one never scored. */
const blockEvidence = {
  b1: { term_compliance: "compliant" as const, voice_score: 62, voice_bar: 90 },
  b2: { term_compliance: "compliant" as const, voice_score: 94, voice_bar: 90 },
};

async function queueWithContexts() {
  const adapter = createMockAdapter(blocks);
  adapter.blockEvidence = blockEvidence;
  const page = await adapter.getPendingReview("demo", "prj-1", undefined);
  return Promise.all(
    page.entries.map(async (e) => {
      const entry: ReviewEntry = {
        id: `${e.item_name}::${e.block_id}::${e.locale}`,
        itemId: e.item_name,
        itemName: e.item_name,
        collectionId: e.collection_id ?? "",
        locale: e.locale,
        block: e.block,
        issues: [],
        termCompliance: e.term_compliance ?? "",
        voiceScore: e.voice_score,
        voiceBar: e.voice_bar,
      };
      const context = await adapter.getReviewContext(
        "demo",
        "prj-1",
        e.item_name,
        e.block_id,
        e.locale,
      );
      return { entry, context };
    }),
  );
}

describe("the mock's review queue and review context", () => {
  it("reads one voice score for both, for every seeded block", async () => {
    const pairs = await queueWithContexts();
    expect(pairs).toHaveLength(3);
    for (const { entry, context } of pairs) {
      expect(context.voice_score).toBe(entry.voiceScore);
      expect(context.voice_bar).toBe(entry.voiceBar);
    }
  });

  it("flags a block in the verdict exactly when the context is below its bar", async () => {
    for (const { entry, context } of await queueWithContexts()) {
      const contextBelow =
        context.voice_score !== undefined && context.voice_score < (context.voice_bar ?? 0);
      expect(isBelowVoiceBar(entry)).toBe(contextBelow);
      expect(entryBlockers(entry).includes("voice")).toBe(contextBelow);
    }
  });

  it("carries voice findings only where the score misses the bar", async () => {
    for (const { entry, context } of await queueWithContexts()) {
      expect((context.judgement.findings ?? []).length > 0).toBe(isBelowVoiceBar(entry));
    }
  });

  it("seeds one block that misses only the voice bar and one that clears every bar", async () => {
    const byBlock = new Map(
      (await queueWithContexts()).map(({ entry, context }) => [entry.block.id, { entry, context }]),
    );
    const low = byBlock.get("b1");
    const high = byBlock.get("b2");
    const unscored = byBlock.get("b3");

    expect(entryVerdict(low!.entry)).toBe("failing");
    expect(entryBlockers(low!.entry)).toEqual(["voice"]);
    expect(entryVerdict(high!.entry)).toBe("passing");
    // An unscored block is below nothing, so the server applies no voice bar to
    // it and neither does the queue.
    expect(unscored!.context.voice_score).toBeUndefined();
    expect(entryVerdict(unscored!.entry)).toBe("passing");
  });
});

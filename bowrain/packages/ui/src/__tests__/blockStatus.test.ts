/**
 * Unit tests for the per-locale block-status reads (Target.Status ladder) and
 * the legacy block-global `properties["translation-status"]` read fallback.
 */
import { describe, it, expect } from "vite-plus/test";

import {
  captureTargetStatus,
  getBlockStatus,
  getTargetStatus,
  getTargetText,
  rollbackTargetStatus,
  statusAfterEdit,
  withTargetEntry,
  withTargetStatus,
} from "../components/editor/blockStatus";
import type { BlockInfo, TargetEntry } from "../types/api";

function makeBlock(
  targets: Record<string, TargetEntry>,
  properties: Record<string, string> = {},
): BlockInfo {
  return {
    id: "b1",
    source: "Hello",
    targets,
    translatable: true,
    has_spans: false,
    properties,
  };
}

describe("getTargetText", () => {
  it("reads a bare-string entry (legacy payload shape)", () => {
    expect(getTargetText(makeBlock({ fr: "Bonjour" }), "fr")).toBe("Bonjour");
  });

  it("reads the text of an object entry", () => {
    expect(getTargetText(makeBlock({ fr: { text: "Bonjour", status: "reviewed" } }), "fr")).toBe(
      "Bonjour",
    );
  });

  it("returns empty for a missing locale and an object without text", () => {
    expect(getTargetText(makeBlock({ fr: "Bonjour" }), "de")).toBe("");
    expect(getTargetText(makeBlock({ fr: { status: "reviewed" } }), "fr")).toBe("");
  });
});

describe("getTargetStatus", () => {
  it("reads the per-locale status of an object entry", () => {
    const block = makeBlock({ fr: { text: "Bonjour", status: "reviewed" } });
    expect(getTargetStatus(block, "fr")).toBe("reviewed");
  });

  it("is empty for bare-string entries and missing locales", () => {
    const block = makeBlock({ fr: "Bonjour" });
    expect(getTargetStatus(block, "fr")).toBe("");
    expect(getTargetStatus(block, "de")).toBe("");
  });
});

describe("getBlockStatus — per-locale Target.Status", () => {
  it("scopes review status to the locale it was set for", () => {
    const block = makeBlock({
      fr: { text: "Bonjour", status: "reviewed" },
      de: { text: "Hallo" },
    });
    expect(getBlockStatus(block, "fr")).toBe("reviewed");
    // Reviewing fr must not leak into de.
    expect(getBlockStatus(block, "de")).toBe("translated");
  });

  it("maps the ladder: draft, translated, reviewed, signed-off", () => {
    expect(getBlockStatus(makeBlock({ fr: { text: "x", status: "draft" } }), "fr")).toBe("draft");
    expect(getBlockStatus(makeBlock({ fr: { text: "x", status: "translated" } }), "fr")).toBe(
      "translated",
    );
    expect(getBlockStatus(makeBlock({ fr: { text: "x", status: "reviewed" } }), "fr")).toBe(
      "reviewed",
    );
    expect(getBlockStatus(makeBlock({ fr: { text: "x", status: "signed-off" } }), "fr")).toBe(
      "reviewed",
    );
  });

  it("prefers the per-locale status over the legacy block-global property", () => {
    const block = makeBlock(
      { fr: { text: "Bonjour", status: "translated" } },
      { "translation-status": "reviewed" },
    );
    expect(getBlockStatus(block, "fr")).toBe("translated");
  });

  it("falls back to the legacy property when no per-locale status exists", () => {
    // A block written before per-locale status: bare-string target + property.
    const reviewed = makeBlock({ fr: "Bonjour" }, { "translation-status": "reviewed" });
    expect(getBlockStatus(reviewed, "fr")).toBe("reviewed");
    const draft = makeBlock({ fr: "Bonjour" }, { "translation-status": "draft" });
    expect(getBlockStatus(draft, "fr")).toBe("draft");
  });

  it("derives not-started / draft-by-origin / translated without any status", () => {
    expect(getBlockStatus(makeBlock({}), "fr")).toBe("not-started");
    expect(
      getBlockStatus(makeBlock({ fr: "Bonjour" }, { "translation-origin": "machine" }), "fr"),
    ).toBe("draft");
    expect(
      getBlockStatus(makeBlock({ fr: "Bonjour" }, { "translation-origin": "pseudo" }), "fr"),
    ).toBe("draft");
    expect(getBlockStatus(makeBlock({ fr: "Bonjour" }), "fr")).toBe("translated");
  });

  it("treats a status entry without target text as not-started (phantom optimistic writes)", () => {
    // A never-rolled-back optimistic write on an untranslated block would
    // otherwise fabricate a Translated/Draft badge with no translation.
    expect(getBlockStatus(makeBlock({ fr: { text: "", status: "translated" } }), "fr")).toBe(
      "not-started",
    );
    expect(getBlockStatus(makeBlock({ fr: { text: "", status: "draft" } }), "fr")).toBe(
      "not-started",
    );
  });

  it("ignores the legacy block-global property for locales with no target text", () => {
    // Pre-migration block: the block-GLOBAL legacy flag plus a target in fr
    // only. de has no translation at all and must not read as reviewed —
    // convergence counts nothing for de, and the UI must agree with coverage.
    const block = makeBlock({ fr: "Bonjour" }, { "translation-status": "reviewed" });
    expect(getBlockStatus(block, "fr")).toBe("reviewed");
    expect(getBlockStatus(block, "de")).toBe("not-started");
  });
});

describe("statusAfterEdit — an edit invalidates a stale review decision", () => {
  it("demotes reviewed/signed-off to translated when the text changes", () => {
    const reviewed = makeBlock({ fr: { text: "Bonjour", status: "reviewed" } });
    expect(statusAfterEdit(reviewed, "fr", "Salut")).toBe("translated");
    const signedOff = makeBlock({ fr: { text: "Bonjour", status: "signed-off" } });
    expect(statusAfterEdit(signedOff, "fr", "Salut")).toBe("translated");
  });

  it("keeps the status when re-saving identical content", () => {
    const reviewed = makeBlock({ fr: { text: "Bonjour", status: "reviewed" } });
    expect(statusAfterEdit(reviewed, "fr", "Bonjour")).toBe("reviewed");
  });

  it("leaves rungs at or below translated alone", () => {
    expect(
      statusAfterEdit(makeBlock({ fr: { text: "Bonjour", status: "draft" } }), "fr", "Salut"),
    ).toBe("draft");
    expect(
      statusAfterEdit(makeBlock({ fr: { text: "Bonjour", status: "translated" } }), "fr", "Salut"),
    ).toBe("translated");
    expect(statusAfterEdit(makeBlock({ fr: "Bonjour" }), "fr", "Salut")).toBe("");
  });
});

describe("withTargetStatus / withTargetEntry — optimistic writes", () => {
  it("writes the per-locale object shape, preserving the target text", () => {
    const block = makeBlock({ fr: "Bonjour", de: "Hallo" });
    const next = withTargetStatus(block, "fr", "reviewed");
    expect(next.targets["fr"]).toEqual({ text: "Bonjour", status: "reviewed" });
    // Other locales and the original block are untouched.
    expect(next.targets["de"]).toBe("Hallo");
    expect(block.targets["fr"]).toBe("Bonjour");
    // Nothing writes the legacy block-global property anymore.
    expect(next.properties["translation-status"]).toBeUndefined();
  });

  it("round-trips a rollback via withTargetEntry", () => {
    const block = makeBlock({ fr: "Bonjour" });
    const prevEntry = block.targets["fr"];
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    const rolledBack = withTargetEntry(optimistic, "fr", prevEntry);
    expect(rolledBack.targets["fr"]).toBe("Bonjour");
    expect(getBlockStatus(rolledBack, "fr")).toBe("translated");
  });

  it("removes the entry when rolling back an entry that did not exist", () => {
    const block = makeBlock({});
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    expect(optimistic.targets["fr"]).toEqual({ text: "", status: "reviewed" });
    const rolledBack = withTargetEntry(optimistic, "fr", undefined);
    expect("fr" in rolledBack.targets).toBe(false);
    expect(getBlockStatus(rolledBack, "fr")).toBe("not-started");
  });
});

describe("captureTargetStatus / rollbackTargetStatus — status-only rollback", () => {
  it("restores the previous status while preserving the current text", () => {
    const block = makeBlock({ fr: { text: "Bonjour", status: "translated" } });
    const snapshot = captureTargetStatus(block, "fr");
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    // A save lands while the review call is in flight.
    const saved = withTargetEntry(optimistic, "fr", "Salut");
    const rolledBack = rollbackTargetStatus(saved, "fr", snapshot);
    // The concurrently saved text survives; only the status rolls back.
    expect(getTargetText(rolledBack, "fr")).toBe("Salut");
    expect(getTargetStatus(rolledBack, "fr")).toBe("translated");
  });

  it("captures a bare-string entry as existed with empty status", () => {
    const block = makeBlock({ fr: "Bonjour" });
    const snapshot = captureTargetStatus(block, "fr");
    expect(snapshot).toEqual({ existed: true, status: "" });
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    const rolledBack = rollbackTargetStatus(optimistic, "fr", snapshot);
    expect(getTargetText(rolledBack, "fr")).toBe("Bonjour");
    expect(getBlockStatus(rolledBack, "fr")).toBe("translated");
  });

  it("removes the entry when it did not exist and no text appeared since", () => {
    const block = makeBlock({});
    const snapshot = captureTargetStatus(block, "fr");
    expect(snapshot).toEqual({ existed: false, status: "" });
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    const rolledBack = rollbackTargetStatus(optimistic, "fr", snapshot);
    expect("fr" in rolledBack.targets).toBe(false);
    expect(getBlockStatus(rolledBack, "fr")).toBe("not-started");
  });

  it("keeps text that appeared after an absent-entry capture", () => {
    const block = makeBlock({});
    const snapshot = captureTargetStatus(block, "fr");
    const optimistic = withTargetStatus(block, "fr", "reviewed");
    const saved = withTargetEntry(optimistic, "fr", "Salut");
    const rolledBack = rollbackTargetStatus(saved, "fr", snapshot);
    expect(getTargetText(rolledBack, "fr")).toBe("Salut");
    expect(getTargetStatus(rolledBack, "fr")).toBe("");
  });
});

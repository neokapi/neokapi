import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "./testUtils";

import { ContextDigestView } from "../components/ContextDigest";
import {
  CONTEXT_DIGEST,
  EMPTY_DIGEST,
  LAST_LOOKED,
  QUIET_DIGEST,
} from "../stories/fixtures/contextDigest";
import type { ContextDigest } from "../types/api";

function renderDigest(digest: ContextDigest = CONTEXT_DIGEST, extra: Record<string, unknown> = {}) {
  const handlers = {
    onKeep: vi.fn(),
    onKeepGroup: vi.fn(),
    onDrop: vi.fn(),
    onChoose: vi.fn(),
    onRevert: vi.fn(),
    onOpenFile: vi.fn(),
  };
  render(<ContextDigestView digest={digest} since={LAST_LOOKED} {...handlers} {...extra} />);
  return handlers;
}

const slot = (name: string) => document.querySelector(`[data-slot='${name}']`);
const all = (name: string) => Array.from(document.querySelectorAll(`[data-slot='${name}']`));

describe("the context digest", () => {
  it("puts what needs a person first and the numbers last", () => {
    renderDigest();
    const order = [
      "digest-conflicts",
      "digest-established",
      "digest-suggested",
      "digest-drift",
      "digest-numbers",
    ]
      .map((name) => slot(name))
      .filter(Boolean);
    expect(order).toHaveLength(5);
    for (let i = 1; i < order.length; i++) {
      expect(
        order[i - 1]!.compareDocumentPosition(order[i]!) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
    expect(slot("digest-numbers")?.textContent).toContain("kapi knows 23 rules for Fernwell");
  });

  it("states a rule as a sentence with the quote and the file it was seen in", () => {
    renderDigest();
    const item = all("digest-item").find((el) => el.textContent?.includes("Quickcast"));
    expect(item?.textContent).toContain("Write Quickcast, not");
    expect(item?.textContent).toContain("Try Quick cast for your next session.");
    expect(item?.textContent).toContain("docs/intro.md");
    expect(item?.textContent).toContain("seen in 2 sessions");
  });

  it("gives the project's own words back beside a rule", () => {
    renderDigest();
    expect(slot("digest-usage")?.textContent).toBe(
      'docs/ says "studio" 41 times and "business" twice',
    );
  });

  it("choosing a side of a conflict chooses, and keeping a suggestion keeps", () => {
    const h = renderDigest();
    const conflict = slot("digest-conflicts")!;
    fireEvent.click(conflict.querySelector("[data-slot='keep-item']")!);
    expect(h.onChoose).toHaveBeenCalledTimes(1);
    expect(h.onKeep).not.toHaveBeenCalled();

    const suggested = slot("digest-suggested")!;
    fireEvent.click(suggested.querySelector("[data-slot='keep-item']")!);
    expect(h.onKeep).toHaveBeenCalledTimes(1);
  });

  it("changes a rule before keeping it", () => {
    const h = renderDigest();
    const suggested = slot("digest-suggested")!;
    fireEvent.click(suggested.querySelector("[data-slot='change-item']")!);
    const input = suggested.querySelector("[data-slot='digest-change'] input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "QuickCast Pro" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(h.onKeep).toHaveBeenCalledWith(
      expect.objectContaining({ theme: "names" }),
      "QuickCast Pro",
    );
  });

  it("reverts an established rule and opens the file a suggestion names", () => {
    const h = renderDigest();
    fireEvent.click(slot("digest-established")!.querySelector("[data-slot='revert-item']")!);
    expect(h.onRevert).toHaveBeenCalledTimes(1);
    fireEvent.click(slot("digest-suggested")!.querySelector("[data-slot='open-file']")!);
    expect(h.onOpenFile).toHaveBeenCalledWith("app/strings/en.json");
  });

  it("keeps a group with one key", () => {
    const h = renderDigest(CONTEXT_DIGEST, { keyboard: true });
    // The cursor starts on the first conflict side; walk to a suggestion.
    for (let i = 0; i < 4; i++) fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: "g" });
    expect(h.onKeepGroup).toHaveBeenCalledTimes(1);
  });

  it("says nothing is new, and keeps what the person already saw under Earlier", () => {
    renderDigest(QUIET_DIGEST);
    expect(slot("digest-quiet")?.textContent).toMatch(/^Nothing new since /);
    expect(all("digest-earlier").length).toBeGreaterThan(0);
    expect(document.body.textContent).not.toMatch(/\b0 pending\b/);
  });

  it("offers no decisions without a checkout", () => {
    renderDigest(CONTEXT_DIGEST, { canDecide: false, onOpenFile: undefined });
    expect(all("digest-actions")).toHaveLength(0);
    expect(all("open-file")).toHaveLength(0);
  });

  it("invites recording on a project nobody has recorded anything about", () => {
    renderDigest(EMPTY_DIGEST, { since: undefined });
    expect(slot("digest-empty")).not.toBeNull();
  });
});

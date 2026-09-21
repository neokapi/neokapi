import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "./testUtils";

import { ContextWidenDialog } from "../components/ContextWidenDialog";
import { ContextRevertDialog } from "../components/ContextRevertDialog";
import { IN_FORCE } from "../stories/fixtures/contextFeed";
import type { ContextWidenPreview } from "../types/api";

const TO_WORKSPACE: ContextWidenPreview = {
  to: "workspace",
  from: { level: "project", describe: "project brand=kapimart" },
  scope: { level: "workspace", describe: "workspace brand=kapimart" },
  rule: { kind: "voice", term: "utilise", replacement: "use" },
  projects: [
    { project_key: "kapimart", project_name: "KapiMart", current: true, checked_out: true },
    { project_key: "bowmart", project_name: "BowMart", current: false, checked_out: true },
    { project_key: "archive", project_name: "Archive", current: false, checked_out: false },
  ],
  points: [],
  content_impact: false,
};

const PAST_AN_AXIS: ContextWidenPreview = {
  to: "product",
  from: { level: "project", describe: "project product=store" },
  scope: { level: "project", describe: "project" },
  rule: { kind: "voice", term: "utilise", replacement: "use" },
  projects: [],
  points: [
    {
      ref: "marketing/web",
      label: "marketing/web",
      coordinates: { product: "marketing", channel: "web" },
      collections: ["campaigns"],
    },
  ],
  content_impact: false,
};

describe("the widen preview", () => {
  it("names every project a workspace-wide rule would answer in", () => {
    render(
      <ContextWidenDialog
        entry={IN_FORCE}
        to="workspace"
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        preview={TO_WORKSPACE}
      />,
    );
    const projects = document.querySelector("[data-slot='widen-projects']");
    expect(projects?.textContent).toContain("KapiMart");
    expect(projects?.textContent).toContain("BowMart");
    expect(projects?.textContent).toContain("Archive");
    expect(projects?.textContent).toContain("no copy on this machine");
    expect(document.body.textContent).toContain("It would newly answer in 2 other projects");
  });

  it("says plainly that it has not counted the content the rule touches", () => {
    render(
      <ContextWidenDialog
        entry={IN_FORCE}
        to="workspace"
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        preview={TO_WORKSPACE}
      />,
    );
    expect(document.querySelector("[data-slot='widen-impact']")?.textContent).toContain(
      "How much content it touches is not counted here",
    );
  });

  it("lists the recipe's points for a widening past one axis", () => {
    render(
      <ContextWidenDialog
        entry={IN_FORCE}
        to="product"
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        preview={PAST_AN_AXIS}
      />,
    );
    const points = document.querySelector("[data-slot='widen-points']");
    expect(points?.textContent).toContain("marketing/web");
    expect(points?.textContent).toContain("campaigns");
    expect(document.querySelector("[data-slot='widen-projects']")).toBeNull();
  });

  it("widens only once the person accepts", () => {
    const onConfirm = vi.fn();
    render(
      <ContextWidenDialog
        entry={IN_FORCE}
        to="workspace"
        onClose={vi.fn()}
        onConfirm={onConfirm}
        preview={TO_WORKSPACE}
      />,
    );
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(document.querySelector("[data-slot='widen-confirm']") as HTMLElement);
    expect(onConfirm).toHaveBeenCalledWith(IN_FORCE, "workspace");
  });
});

describe("the undo confirmation", () => {
  it("names how many operations a session revert undoes", () => {
    render(
      <ContextRevertDialog
        request={{ project: "kapimart", session: "sess-1" }}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        scope={{
          session: "sess-1",
          operations: 4,
          rules: ['term "sign in", use "log in"'],
          subjects: ['term "sign in", use "log in"', 'voice "utilise", use "use"'],
        }}
      />,
    );
    expect(document.querySelector("[data-slot='revert-count']")?.textContent).toContain(
      "This undoes 4 operations.",
    );
    expect(document.querySelector("[data-slot='revert-count']")?.textContent).toContain(
      "1 rule is in force",
    );
    expect(document.querySelector("[data-slot='revert-subjects']")?.textContent).toContain(
      "utilise",
    );
  });

  it("undoes only once the person accepts", () => {
    const onConfirm = vi.fn();
    const request = { project: "kapimart", id: "9" };
    render(
      <ContextRevertDialog
        request={request}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        scope={{ operations: 1, rules: [], subjects: [] }}
      />,
    );
    expect(document.querySelector("[data-slot='revert-count']")?.textContent).toContain(
      "This undoes 1 operation.",
    );
    fireEvent.click(document.querySelector("[data-slot='revert-confirm']") as HTMLElement);
    expect(onConfirm).toHaveBeenCalledWith(request);
  });
});

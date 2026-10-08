import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "./testUtils";

import { ContextWidenDialog, widenRuleOfEntry } from "../components/ContextWidenDialog";
import { ContextResetDialog } from "../components/ContextResetDialog";
import { IN_FORCE } from "../stories/fixtures/contextFeed";
import type { ContextWidenPreview } from "../types/api";

const RULE = widenRuleOfEntry(IN_FORCE);

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
        rule={RULE}
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
        rule={RULE}
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
        rule={RULE}
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
        rule={RULE}
        to="workspace"
        onClose={vi.fn()}
        onConfirm={onConfirm}
        preview={TO_WORKSPACE}
      />,
    );
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(document.querySelector("[data-slot='widen-confirm']") as HTMLElement);
    expect(onConfirm).toHaveBeenCalledWith(RULE, "workspace");
  });
});

describe("the reset confirmation", () => {
  it("names what resetting to before a session sets aside", () => {
    render(
      <ContextResetDialog
        request={{ project: "kapimart", before: "sess-1" }}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        scope={{
          before: "sess-1",
          set_aside: 4,
          decisions: 1,
          restored: 0,
          rules: ['term "sign in", use "log in"'],
          subjects: ['term "sign in", use "log in"', 'voice "utilise", use "use"'],
        }}
      />,
    );
    const count = document.querySelector("[data-slot='reset-count']")?.textContent;
    expect(count).toContain("This sets aside 4 suggestions and rules.");
    expect(count).toContain("1 rule is in force");
    expect(count).toContain("1 later decision is set aside");
    expect(document.querySelector("[data-slot='reset-subjects']")?.textContent).toContain(
      "utilise",
    );
    expect(document.querySelector("[data-slot='reset-dialog']")?.textContent).toContain(
      "A later reset to before this one brings it back.",
    );
  });

  it("resets only once the person accepts", () => {
    const onConfirm = vi.fn();
    const request = { project: "kapimart", before: "sess-1" };
    render(
      <ContextResetDialog
        request={request}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        scope={{
          before: "sess-1",
          set_aside: 1,
          decisions: 0,
          restored: 0,
          rules: [],
          subjects: [],
        }}
      />,
    );
    expect(document.querySelector("[data-slot='reset-count']")?.textContent).toContain(
      "This sets aside 1 suggestion or rule.",
    );
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(document.querySelector("[data-slot='reset-confirm']") as HTMLElement);
    expect(onConfirm).toHaveBeenCalledWith(request);
  });
});

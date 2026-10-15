import { describe, it, expect, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen } from "./testUtils";

import { WhatsNew, WorkspaceNumbers, themesOf } from "../components/WhatsNew";
import { formatLook } from "../components/ContextDigest";
import {
  ACME,
  FIELD_GUIDE,
  HANDBOOK,
  KAPIMART,
  NOTHING,
  ONE_WORKING,
  SEVERAL,
} from "../stories/fixtures/standings";

const all = (name: string) => Array.from(document.querySelectorAll(`[data-slot='${name}']`));

describe("the workspace numbers", () => {
  it("states the rules, the projects and the suggestions in one sentence", () => {
    render(<WorkspaceNumbers standings={SEVERAL} />);
    expect(
      screen.getByText(
        "kapi knows 61 rules across 4 projects. 11 suggestions are advising.",
      ),
    ).toBeInTheDocument();
  });

  it("says who is working where while a session is working", () => {
    render(<WorkspaceNumbers standings={ONE_WORKING} />);
    expect(
      screen.getByText("claude-code is working in Acme Help Center now."),
    ).toBeInTheDocument();
  });

  it("says nothing is recorded when no project holds a rule or a suggestion", () => {
    render(<WorkspaceNumbers standings={NOTHING} />);
    expect(screen.getByText("Nothing is recorded yet.")).toBeInTheDocument();
  });

  it("uses the singular for one rule in one project", () => {
    render(<WorkspaceNumbers standings={[{ ...FIELD_GUIDE, rules: 1 }]} />);
    expect(screen.getByText("kapi knows 1 rule across 1 project.")).toBeInTheDocument();
  });
});

describe("what is new across the workspace", () => {
  it("lists the projects with news, newest session first", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const rows = all("whats-new-row").map((r) => r.getAttribute("data-project"));
    // Field Guide has nothing new, so it has no row.
    expect(rows).toEqual([ACME.project, KAPIMART.project, HANDBOOK.project]);
  });

  it("reads each row word for word", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const sentences = all("whats-new-sentence").map((p) => p.textContent);
    expect(sentences).toEqual([
      `4 new since ${formatLook(ACME.since!)}. claude-code noticed 3 names and 1 word.`,
      `2 new since ${formatLook(KAPIMART.since!)}. codex noticed 1 name and 1 word. 1 needs you.`,
      "4 new. cursor noticed 2 names, 1 word and 1 note.",
    ]);
  });

  it("reads Decide for a project that needs a person and Review otherwise", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    expect(all("whats-new-open").map((b) => b.textContent)).toEqual([
      "Review",
      "Decide",
      "Review",
    ]);
  });

  it("pulses the dot only while the session is working", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const dots = all("whats-new-dot");
    expect(dots[0].className).toContain("motion-safe:animate-pulse");
    expect(dots[1].className).not.toContain("animate-pulse");
  });

  it("opens the project from its button", async () => {
    const onOpen = vi.fn();
    render(<WhatsNew standings={SEVERAL} onOpen={onOpen} />);
    await userEvent.click(screen.getByRole("button", { name: "Decide" }));
    expect(onOpen).toHaveBeenCalledWith(KAPIMART.project);
  });

  it("moves over the rows with j and k and opens with Enter", async () => {
    const onOpen = vi.fn();
    render(<WhatsNew standings={SEVERAL} onOpen={onOpen} />);
    await userEvent.keyboard("j");
    await userEvent.keyboard("j");
    await userEvent.keyboard("k");
    await userEvent.keyboard("{Enter}");
    expect(onOpen).toHaveBeenCalledWith(KAPIMART.project);
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(onOpen).toHaveBeenLastCalledWith(HANDBOOK.project);
  });

  it("leaves the keys to an input that has focus", async () => {
    const onOpen = vi.fn();
    render(
      <>
        <input aria-label="search" />
        <WhatsNew standings={SEVERAL} onOpen={onOpen} />
      </>,
    );
    await userEvent.click(screen.getByLabelText("search"));
    await userEvent.keyboard("j{Enter}");
    expect(onOpen).not.toHaveBeenCalled();
    expect(screen.getByLabelText("search")).toHaveValue("j");
  });

  it("takes no keys when told not to", async () => {
    const onOpen = vi.fn();
    render(<WhatsNew standings={SEVERAL} onOpen={onOpen} keyboard={false} />);
    await userEvent.keyboard("{Enter}");
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("is absent when no project has news", () => {
    render(<WhatsNew standings={[FIELD_GUIDE]} onOpen={vi.fn()} />);
    expect(all("whats-new")).toHaveLength(0);
  });
});

describe("the themes a session noticed", () => {
  it("follows the digest's order and plural forms", () => {
    expect(themesOf({ ...ACME.latest!, new_by_theme: { words: 1 } })).toBe("1 word");
    expect(themesOf({ ...ACME.latest!, new_by_theme: { memory: 2, names: 1 } })).toBe(
      "1 name and 2 wordings",
    );
    expect(themesOf({ ...ACME.latest!, new_by_theme: {} })).toBe("");
  });
});

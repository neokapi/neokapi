import { describe, it, expect, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen } from "./testUtils";

import { WhatsNew, WorkspaceNumbers, markActor, newsChips } from "../components/WhatsNew";
import {
  ACME,
  FIELD_GUIDE,
  HANDBOOK,
  KAPIMART,
  NOTHING,
  ONE_WORKING,
  SEVERAL,
} from "../stories/fixtures/standings";

const all = (name: string, root: ParentNode = document) =>
  Array.from(root.querySelectorAll(`[data-slot='${name}']`));

/** Each count as assistive technology reads it, with the tone it is drawn in. */
function stats() {
  return all("stat").map((li) => ({
    phrase: li.querySelector(".sr-only")?.textContent,
    tone: li.getAttribute("data-tone"),
  }));
}

/** Each chip on a row as its kind and its text. */
function chipsOf(row: Element) {
  return all("whats-new-chip", row).map((c) => [c.getAttribute("data-kind"), c.textContent]);
}

describe("the workspace numbers", () => {
  it("draws the projects, the rules, the terms, the suggestions and what needs you as labelled counts", () => {
    render(<WorkspaceNumbers standings={SEVERAL} />);
    expect(all("workspace-numbers")).toHaveLength(1);
    expect(stats()).toEqual([
      { phrase: "4 projects", tone: "default" },
      { phrase: "61 rules", tone: "default" },
      { phrase: "260 terms", tone: "default" },
      { phrase: "11 suggestions", tone: "default" },
      { phrase: "1 needs you", tone: "warning" },
    ]);
  });

  it("counts a project's terms apart from its rules", () => {
    render(<WorkspaceNumbers standings={[{ ...NOTHING[0], terms: 100 }]} />);
    expect(stats().map((s) => s.phrase)).toEqual([
      "1 project",
      "0 rules",
      "100 terms",
      "0 suggestions",
      "0 need you",
    ]);
  });

  it("draws needs you in the muted ink when nothing needs a person", () => {
    render(<WorkspaceNumbers standings={ONE_WORKING} />);
    expect(stats()[4]).toEqual({ phrase: "0 need you", tone: "muted" });
  });

  it("counts the workspace's projects when it is told how many there are", () => {
    render(<WorkspaceNumbers standings={ONE_WORKING} projects={3} />);
    expect(stats()[0].phrase).toBe("3 projects");
  });

  it("uses the singular for one of each", () => {
    render(<WorkspaceNumbers standings={[{ ...FIELD_GUIDE, rules: 1, suggested: 1, terms: 1 }]} />);
    expect(stats().map((s) => s.phrase)).toEqual([
      "1 project",
      "1 rule",
      "1 term",
      "1 suggestion",
      "0 need you",
    ]);
  });

  it("writes no sentence from a number", () => {
    render(<WorkspaceNumbers standings={SEVERAL} />);
    expect(screen.queryByText(/kapi knows/)).not.toBeInTheDocument();
    expect(screen.queryByText(/are advising/)).not.toBeInTheDocument();
  });
});

describe("what is new across the workspace", () => {
  it("lists the projects with news, newest session first", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const rows = all("whats-new-row").map((r) => r.getAttribute("data-project"));
    // Field Guide has nothing new, so it has no row.
    expect(rows).toEqual([ACME.project, KAPIMART.project, HANDBOOK.project]);
  });

  it("marks each row with whoever noticed something most recently", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    expect(all("whats-new-actor").map((m) => m.getAttribute("data-actor"))).toEqual([
      "claude-code",
      "codex",
      "cursor",
    ]);
    expect(screen.getByRole("img", { name: "Claude Code" })).toBeInTheDocument();
  });

  it("puts a chip on each row for each kind of thing noticed, in the digest's order", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    expect(all("whats-new-row").map(chipsOf)).toEqual([
      [
        ["name", "3 names"],
        ["word", "1 word"],
      ],
      [
        ["name", "1 name"],
        ["word", "1 word"],
        ["conflict", "1 needs you"],
      ],
      [
        ["name", "2 names"],
        ["word", "1 word"],
        ["note", "1 note"],
      ],
    ]);
  });

  it("draws the chip that needs a person in the warning ink, and no other", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const warned = all("whats-new-chip").filter(
      (c) => c.getAttribute("data-variant") === "warning",
    );
    expect(warned.every((c) => c.className.includes("text-warning-ink"))).toBe(true);
    expect(warned.map((c) => c.getAttribute("data-kind"))).toEqual(["conflict"]);
  });

  it("writes no sentence about a row", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    expect(all("whats-new-sentence")).toHaveLength(0);
    expect(screen.queryByText(/noticed/)).not.toBeInTheDocument();
    expect(screen.queryByText(/new since/)).not.toBeInTheDocument();
  });

  it("shows a working chip only on the row whose assistant is working now", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const working = all("workspace-working");
    expect(working).toHaveLength(1);
    expect(working[0].textContent).toBe("claude-code is working");
    expect(working[0].closest("[data-slot='whats-new-row']")?.getAttribute("data-project")).toBe(
      ACME.project,
    );
    expect(working[0].querySelector(".motion-safe\\:animate-pulse")).not.toBeNull();
  });

  it("keeps a row for a project an assistant is working in with nothing new yet", () => {
    const busy = {
      ...FIELD_GUIDE,
      latest: { ...ACME.latest!, new_by_theme: undefined },
    };
    render(<WhatsNew standings={[busy]} onOpen={vi.fn()} />);
    expect(all("whats-new-row")).toHaveLength(1);
    expect(all("workspace-working")).toHaveLength(1);
    expect(all("whats-new-chip")).toHaveLength(0);
  });

  it("keeps a project whose suggestions still wait after the marker moved", () => {
    // Opening a project on its What's new moves the marker at once, so a
    // fresh sample would otherwise vanish from the home with its suggestions
    // still undecided.
    const seen = {
      ...ACME,
      new: 0,
      since: new Date().toISOString(),
      suggested: 5,
      latest: { ...ACME.latest!, working: false, new_by_theme: undefined },
    };
    render(<WhatsNew standings={[seen]} onOpen={vi.fn()} />);
    expect(all("whats-new-row").map(chipsOf)).toEqual([[["suggested", "5 suggestions"]]]);
  });

  it("reads Decide for a project that needs a person and Review otherwise", () => {
    render(<WhatsNew standings={SEVERAL} onOpen={vi.fn()} />);
    const buttons = all("whats-new-open");
    expect(buttons.map((b) => b.textContent)).toEqual(["Review", "Decide", "Review"]);
    // Row actions are outline: the page keeps its one primary button.
    expect(buttons.every((b) => b.getAttribute("data-variant") === "outline")).toBe(true);
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

  it("says in one line that nothing is new when projects hold rules and nothing waits", () => {
    render(<WhatsNew standings={[FIELD_GUIDE]} onOpen={vi.fn()} />);
    expect(all("whats-new-row")).toHaveLength(0);
    expect(all("whats-new-empty").map((p) => p.textContent)).toEqual([
      "Nothing new since you last looked.",
    ]);
  });

  it("says in one line that nothing is recorded when no project holds anything", () => {
    render(<WhatsNew standings={NOTHING} onOpen={vi.fn()} />);
    expect(all("whats-new-empty").map((p) => p.textContent)).toEqual([
      "Nothing is recorded yet. What an assistant notices in a project appears here.",
    ]);
  });

  it("draws nothing while the standings are being read", () => {
    render(<WhatsNew standings={null} onOpen={vi.fn()} />);
    expect(all("whats-new")).toHaveLength(0);
  });
});

describe("the chips a project's row carries", () => {
  it("counts each kind the latest session noticed, with the plural forms", () => {
    const chips = newsChips({
      ...ACME,
      latest: { ...ACME.latest!, new_by_theme: { memory: 2, names: 1 } },
    });
    expect(chips.map((c) => [c.kind, c.label])).toEqual([
      ["name", "1 name"],
      ["memory", "2 wordings"],
    ]);
  });

  it("says how much is new when the latest session says nothing about kinds", () => {
    const chips = newsChips({ ...ACME, latest: { ...ACME.latest!, new_by_theme: {} } });
    expect(chips.map((c) => [c.kind, c.label])).toEqual([["new", "4 new"]]);
  });

  it("counts the disagreements last, as the one chip that waits for a person", () => {
    const chips = newsChips({ ...KAPIMART, conflicts: 2 });
    expect(chips[chips.length - 1]).toEqual({
      kind: "conflict",
      count: 2,
      label: "2 need you",
      warning: true,
    });
  });
});

describe("the mark a session is drawn with", () => {
  it("names an agent by its own name and a person as a person", () => {
    expect(markActor({ kind: "agent", name: "claude-code" })).toEqual({ actor: "claude-code" });
    expect(markActor({ kind: "person", name: "Ingrid Solberg" })).toEqual({
      actor: "person",
      name: "Ingrid Solberg",
    });
    expect(markActor({ kind: "tool" })).toEqual({ actor: "tool" });
  });
});

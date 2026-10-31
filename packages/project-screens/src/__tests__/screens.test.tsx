// @vitest-environment jsdom
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { DecideScreen, KeptRulesScreen, MapScreen, ReaderScreen } from "../screens";
import { GROUP_ABOVE } from "../KeptRulesView";
import type { ProjectScreensPort } from "../port";
import {
  DECIDE_IDS,
  DECIDE_VIEW,
  KEPT_RULES,
  MAP_PENDING,
  READER_CLEAN,
  READER_WITH_FIXES,
} from "../stories/fixtures";

/** A port with the three reads and no action: a read-only preview. */
function readOnly(over: Partial<ProjectScreensPort> = {}): ProjectScreensPort {
  return {
    mapSummary: async () => MAP_PENDING,
    readerFile: async () => READER_WITH_FIXES,
    decideView: async () => DECIDE_VIEW,
    ...over,
  };
}

describe("the screens over a read-only port", () => {
  it("draw the Map with no check action", async () => {
    render(<MapScreen port={readOnly()} projectName="Fernwell" navigate={vi.fn()} />);
    expect(await screen.findByRole("heading", { name: "Fernwell" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Run on everything" })).toBeNull();
  });

  it("draw the Reader with no Fix", async () => {
    render(<ReaderScreen port={readOnly()} path="docs/billing.md" navigate={vi.fn()} />);
    expect(await screen.findByRole("heading", { name: "docs/billing.md" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
  });

  it("draw Decide with no Keep, Not a rule or Later", async () => {
    render(<DecideScreen port={readOnly()} keyboard={false} navigate={vi.fn()} />);
    expect(await screen.findByRole("listbox", { name: "Suggestions to decide" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Keep/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Not a rule/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Later/ })).toBeNull();
  });

  it("hide the Map's Open when the host opens no Reader and previews nothing", async () => {
    render(
      <MapScreen
        port={readOnly()}
        projectName="Fernwell"
        navigate={vi.fn()}
        canNavigate={(target) => target.kind !== "reader"}
      />,
    );
    await screen.findByRole("heading", { name: "Fernwell" });
    expect(screen.queryByRole("button", { name: "Open" })).toBeNull();
  });
});

describe("the screens over a port that acts", () => {
  it("run the checks and read the Map again", async () => {
    const mapSummary = vi.fn(async () => MAP_PENDING);
    const runChecks = vi.fn(async () => {});
    render(
      <MapScreen
        port={readOnly({ mapSummary, runChecks })}
        projectName="Fernwell"
        navigate={vi.fn()}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Run on everything" }));
    expect(runChecks).toHaveBeenCalled();
    await waitFor(() => expect(mapSummary).toHaveBeenCalledTimes(2));
  });

  it("apply a fix and draw the file the port answers with", async () => {
    const fixFile = vi.fn(async () => READER_CLEAN);
    render(<ReaderScreen port={readOnly({ fixFile })} path="docs/billing.md" navigate={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "Fix 2 in this file" }));
    expect(fixFile).toHaveBeenCalledWith("docs/billing.md");
    expect(await screen.findByText("3 blocks · 0 findings")).toBeInTheDocument();
  });

  it("record an answer with the id and the form a kept name writes", async () => {
    const decide = vi.fn(async () => ({ answer: "keep" as const, kept: 1, dropped: 0, later: 0 }));
    render(
      <DecideScreen
        port={readOnly({ decide })}
        defaultSelected={`suggested:${DECIDE_IDS.quickcast}`}
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    const pane = await screen.findByRole("radiogroup");
    await userEvent.click(within(pane).getAllByRole("radio")[1]);
    await userEvent.click(screen.getByRole("button", { name: /^Keep/ }));
    expect(decide).toHaveBeenCalledWith(DECIDE_IDS.quickcast, "keep", "Quick cast");
  });

  it("read again when the port reports a change", async () => {
    let changed: ((what: "map" | "reader" | "decide") => void) | undefined;
    const decideView = vi.fn(async () => DECIDE_VIEW);
    render(
      <DecideScreen
        port={readOnly({
          decideView,
          subscribe: (onChange) => {
            changed = onChange;
            return () => {};
          },
        })}
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    act(() => changed?.("map"));
    act(() => changed?.("decide"));
    await waitFor(() => expect(decideView).toHaveBeenCalledTimes(2));
  });

  it("read the questions again once an answer is recorded, on a port that reports no change", async () => {
    let answered = false;
    const decideView = vi.fn(async () =>
      answered
        ? {
            ...DECIDE_VIEW,
            decisions: DECIDE_VIEW.decisions.slice(1),
            rules: DECIDE_VIEW.rules + 1,
          }
        : DECIDE_VIEW,
    );
    const decide = vi.fn(async () => {
      answered = true;
      return { answer: "keep" as const, kept: 1, dropped: 0, later: 0 };
    });
    const { container } = render(
      <DecideScreen port={readOnly({ decideView, decide })} keyboard={false} navigate={vi.fn()} />,
    );
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    await userEvent.click(screen.getByRole("button", { name: /^Keep/ }));
    await waitFor(() => expect(decideView).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(container.querySelector("[data-slot='decide-counts']")).toHaveTextContent(
        `${DECIDE_VIEW.decisions.length - 1} to decide · ${DECIDE_VIEW.rules + 1} kept`,
      ),
    );
  });
});

describe("Kept by", () => {
  it("lists each kept rule with who kept it, the newest first", async () => {
    render(<KeptRulesScreen port={readOnly({ keptRules: async () => KEPT_RULES })} />);
    const rows = await screen.findAllByRole("listitem");
    expect(rows).toHaveLength(KEPT_RULES.length);
    expect(within(rows[0]).getByText(KEPT_RULES[0].rule)).toBeInTheDocument();
    const by = rows.map((r) => r.querySelector("[data-slot='kept-by']")?.textContent);
    // Each row names its keeper after the keeper's mark, and an assistant's session after its name.
    expect(by[0]).toMatch(/you$/);
    expect(by[1]).toMatch(/ada$/);
    expect(by[2]).toMatch(/kapi$/);
    expect(by[3]).toMatch(/claudes3$/);
    expect(document.querySelector("[data-slot='kept-group']")).toBeNull();
  });

  it("draws the newest first whatever order the port answers in", async () => {
    render(
      <KeptRulesScreen port={readOnly({ keptRules: async () => [...KEPT_RULES].reverse() })} />,
    );
    const rows = await screen.findAllByRole("listitem");
    expect(within(rows[0]).getByText(KEPT_RULES[0].rule)).toBeInTheDocument();
    expect(within(rows[3]).getByText(KEPT_RULES[3].rule)).toBeInTheDocument();
  });

  it("splits a long list under a heading per kind, each the newest first", async () => {
    const many = Array.from({ length: GROUP_ABOVE + 1 }, (_, i) => ({
      ...KEPT_RULES[i % 2],
      rule: `Rule ${i}`,
      kept_at: new Date(Date.UTC(2026, 9, 1 + i)).toISOString(),
    }));
    render(<KeptRulesScreen port={readOnly({ keptRules: async () => many })} />);
    await screen.findAllByRole("listitem");
    const groups = Array.from(document.querySelectorAll<HTMLElement>("[data-slot='kept-group']"));
    expect(groups.map((g) => g.dataset.kind)).toEqual(["name", "word"]);
    expect(within(groups[0]).getByRole("heading").textContent).toBe("Name6");
    const words = within(groups[1]).getAllByRole("listitem");
    expect(words).toHaveLength(7);
    expect(words[0].textContent).toContain("Rule 12");
    expect(words[6].textContent).toContain("Rule 0");
  });

  it("draws nothing on a port with no keptRules", () => {
    const { container } = render(<KeptRulesScreen port={readOnly()} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("the Reader on a host that files each fix as a proposal", () => {
  it("draws the proposal under its finding, with a link, and keeps the file it read", async () => {
    const readerFile = vi.fn(async () => READER_WITH_FIXES);
    const fixFinding = vi.fn(async () => ({
      proposed: [{ finding_id: "a1f0-1", proposal: "0d4c2b9e-77aa-4f1e-9c11-5b2f0e0a1d3c" }],
      refused: [],
    }));
    const onOpenProposal = vi.fn();
    const { container } = render(
      <ReaderScreen
        port={readOnly({ readerFile, fixFinding, fixFile: vi.fn() })}
        path="docs/billing.md"
        navigate={vi.fn()}
        onOpenProposal={onOpenProposal}
      />,
    );
    await screen.findByRole("heading", { name: "docs/billing.md" });
    const row = container.querySelector<HTMLElement>(
      "[data-slot='reader-finding'][data-id='a1f0-1']",
    )!;
    await userEvent.click(within(row).getByRole("button", { name: "Fix" }));
    expect(fixFinding).toHaveBeenCalledWith("docs/billing.md", "a1f0-1");
    const link = await within(row).findByRole("button", { name: "Proposed" });
    expect(link).toHaveAttribute("title", "0d4c2b9e-77aa-4f1e-9c11-5b2f0e0a1d3c");
    expect(within(row).queryByRole("button", { name: "Fix" })).toBeNull();
    // Fixing the file would file this fix again, so its action goes.
    expect(screen.queryByRole("button", { name: /in this file/ })).toBeNull();
    await userEvent.click(link);
    expect(onOpenProposal).toHaveBeenCalledWith("0d4c2b9e-77aa-4f1e-9c11-5b2f0e0a1d3c");
    // A filed fix changes nothing until a person approves it, so the file is
    // not read again.
    expect(readerFile).toHaveBeenCalledTimes(1);
  });

  it("says why a fix could not be filed, under its finding", async () => {
    const fixFile = vi.fn(async () => ({
      proposed: [{ finding_id: "a1f0-1", proposal: "p-1" }],
      refused: [{ finding_id: "b2c3-1", reason: "The change was refused." }],
    }));
    const { container } = render(
      <ReaderScreen port={readOnly({ fixFile })} path="docs/billing.md" navigate={vi.fn()} />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Fix 2 in this file" }));
    await waitFor(() =>
      expect(container.querySelector("[data-slot='reader-finding-refused']")).toHaveTextContent(
        "Not fixed: The change was refused.",
      ),
    );
    expect(container.querySelector("[data-slot='reader-proposed']")).toHaveTextContent("Proposed");
  });

  it("draws no file fix action once every fix is filed", async () => {
    const fixFile = vi.fn(async () => ({
      proposed: [
        { finding_id: "a1f0-1", proposal: "p-1" },
        { finding_id: "b2c3-1", proposal: "p-1" },
      ],
      refused: [],
    }));
    render(<ReaderScreen port={readOnly({ fixFile })} path="docs/billing.md" navigate={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "Fix 2 in this file" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /in this file/ })).toBeNull());
  });
});

describe("Decide on a port that assigns questions", () => {
  const assignedView = {
    ...DECIDE_VIEW,
    decisions: DECIDE_VIEW.decisions.map((d) =>
      d.id === DECIDE_IDS.quickcast
        ? { ...d, assignee: { kind: "person" }, assignee_id: "u-me" }
        : d,
    ),
  };
  const members = [
    { id: "u-me", name: "Ada Holt", me: true },
    { id: "u-mira", name: "Mira Holt" },
  ];

  it("assigns through the port by the question's id and reads the questions again", async () => {
    const assign = vi.fn(async () => {});
    const decideView = vi.fn(async () => assignedView);
    render(
      <DecideScreen
        port={readOnly({ decideView, assign, members: async () => members })}
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    await screen.findAllByRole("option");
    const picker = await screen.findByRole("combobox", { name: "Assigned to" });
    await userEvent.click(picker);
    await userEvent.click(await screen.findByRole("option", { name: "Mira Holt" }));
    // The first question is the disagreement, assigned under its first side's id.
    expect(assign).toHaveBeenCalledWith(DECIDE_IDS.store, "u-mira");
    await waitFor(() => expect(decideView).toHaveBeenCalledTimes(2));
  });

  it("opens on the questions assigned to the reader once the members are read", async () => {
    let answer: (list: typeof members) => void = () => {};
    render(
      <DecideScreen
        port={readOnly({
          decideView: async () => assignedView,
          assign: async () => {},
          members: () => new Promise((resolve) => (answer = resolve)),
        })}
        assignedToMe
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    // Until the members are read, which questions are the reader's is not known.
    await waitFor(() =>
      expect(document.querySelector("[data-slot='decide-mine-loading']")).not.toBeNull(),
    );
    expect(screen.queryByText("Nothing assigned to you.")).toBeNull();
    expect(screen.getByRole("button", { name: /Assigned to me/ })).toBeDisabled();
    act(() => answer(members));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    expect(screen.getAllByRole("option")[0].getAttribute("data-id")).toBe(DECIDE_IDS.quickcast);
    expect(screen.getByRole("button", { name: /Assigned to me/ })).toBeEnabled();
  });

  it("shows every question, the filter off, when the members cannot be read", async () => {
    render(
      <DecideScreen
        port={readOnly({
          decideView: async () => assignedView,
          assign: async () => {},
          members: async () => {
            throw new Error("members unavailable");
          },
        })}
        assignedToMe
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(4));
    const toggle = screen.getByRole("button", { name: /Assigned to me/ });
    expect(toggle).toBeDisabled();
    expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  it("draws no assignment on a port with no assign", async () => {
    render(
      <DecideScreen
        port={readOnly({ decideView: async () => assignedView })}
        assignedToMe
        keyboard={false}
        navigate={vi.fn()}
      />,
    );
    await screen.findAllByRole("option");
    expect(document.querySelectorAll("[data-slot='decide-item']")).toHaveLength(4);
    expect(screen.queryByRole("combobox")).toBeNull();
  });
});

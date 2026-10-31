// @vitest-environment jsdom
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { DecideScreen, KeptRulesScreen, MapScreen, ReaderScreen } from "../screens";
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
  it("lists each kept rule with who kept it", async () => {
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
  });

  it("draws nothing on a port with no keptRules", () => {
    const { container } = render(<KeptRulesScreen port={readOnly()} />);
    expect(container).toBeEmptyDOMElement();
  });
});

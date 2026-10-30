import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import { AssistantPanel } from "../components/AssistantPanel";
import { RulesFilesView } from "../components/RulesFilesView";
import { api } from "../hooks/useApi";
import {
  RULES_FILES_NOT_WRITTEN,
  RULES_FILES_PARTLY_WRITTEN,
  RULES_FILES_WRITTEN,
  WRITE_RULES_RESULT,
} from "../stories/fixtures/rulesFiles";
import { BRIEF_FILES, BRIEF_PATH, BRIEF_VIEW } from "../stories/fixtures/agentContext";

afterEach(() => {
  vi.restoreAllMocks();
});

const HELP =
  "kapi writes the rules into AGENTS.md and CLAUDE.md, which an assistant reads as it starts.";

/** The brief's backend: the files the picker offers, and the one answer. */
function stubBrief() {
  vi.spyOn(api, "contextOptions").mockResolvedValue(BRIEF_FILES);
  return vi.spyOn(api, "agentContextAt").mockResolvedValue(BRIEF_VIEW);
}

describe("What your assistant reads", () => {
  it("shows two tabs, opening on the rules files", () => {
    render(<AssistantPanel tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "What your assistant reads",
    );
    expect(screen.queryAllByRole("heading", { level: 2 })).toHaveLength(0);
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Rules files",
      "The brief for a file",
    ]);
    const rules = screen.getByRole("tab", { name: "Rules files" });
    expect(rules).toHaveAttribute("aria-selected", "true");
    // The active style keys on data-state, which a tooltip wrapper would overwrite.
    expect(rules).toHaveAttribute("data-state", "active");
    expect(screen.getByRole("tab", { name: "The brief for a file" })).toHaveAttribute(
      "data-state",
      "inactive",
    );
    expect(document.querySelector("[data-slot='rules-files']")).not.toBeNull();
    expect(document.querySelector("[data-slot='assistant-brief']")).toBeNull();
  });

  it("names no command line and explains nothing under a heading", () => {
    render(<AssistantPanel tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    const panel = document.querySelector<HTMLElement>("[data-slot='assistant-panel']")!;
    expect(panel.textContent).not.toMatch(/The same as/);
    expect(panel.textContent).not.toContain(HELP);
  });

  it("switches to the brief for a file, with the picker and what it needs", async () => {
    stubBrief();
    render(<AssistantPanel tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    await userEvent.click(screen.getByRole("tab", { name: "The brief for a file" }));

    expect(screen.getByRole("tab", { name: "The brief for a file" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(document.querySelector("[data-slot='rules-files']")).toBeNull();
    const brief = document.querySelector<HTMLElement>("[data-slot='assistant-brief']")!;
    expect(within(brief).getByRole("combobox", { name: "Choose a file" })).toBeInTheDocument();
    expect(within(brief).getByText("No file chosen")).toBeInTheDocument();
    expect(brief.textContent).not.toMatch(/the same text/);

    await userEvent.click(screen.getByRole("tab", { name: "Rules files" }));
    expect(document.querySelector("[data-slot='rules-files']")).not.toBeNull();
  });

  it("opens on the brief, with the file chosen, when handed a path", async () => {
    const read = stubBrief();
    render(
      <AssistantPanel
        tabID="t1"
        path={BRIEF_PATH}

        rulesFiles={RULES_FILES_WRITTEN}
      />,
    );
    expect(screen.getByRole("tab", { name: "The brief for a file" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(read).toHaveBeenCalledWith("t1", BRIEF_PATH, 0);
    const text = await screen.findByTestId("agent-context-text");
    expect(text.textContent).toBe(BRIEF_VIEW.text);
    expect(
      within(document.querySelector<HTMLElement>("[data-slot='assistant-brief']")!).getByText(
        "studio",
      ),
    ).toBeInTheDocument();
  });

  it("moves to the brief when a path is handed in later", async () => {
    const read = stubBrief();
    const { rerender } = render(<AssistantPanel tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    expect(screen.getByRole("tab", { name: "Rules files" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    rerender(
      <AssistantPanel
        tabID="t1"
        path={BRIEF_PATH}

        rulesFiles={RULES_FILES_WRITTEN}
      />,
    );
    expect(screen.getByRole("tab", { name: "The brief for a file" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await waitFor(() => expect(read).toHaveBeenCalledWith("t1", BRIEF_PATH, 0));
  });

  it("goes back to the Assistants page", async () => {
    const onBack = vi.fn();
    render(<AssistantPanel tabID="t1" rulesFiles={RULES_FILES_WRITTEN} onBack={onBack} />);
    await userEvent.click(screen.getByRole("button", { name: "Assistants" }));
    expect(onBack).toHaveBeenCalledOnce();
  });
});

describe("rules files", () => {
  it("lists each file with its section, and no sentence or command above them", async () => {
    render(<RulesFilesView tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    const view = document.querySelector<HTMLElement>("[data-slot='rules-files']")!;
    expect(view.textContent).not.toContain(HELP);
    expect(view.textContent).not.toMatch(/kapi context sync/);
    const rows = document.querySelectorAll<HTMLElement>("[data-slot='rules-file']");
    expect(Array.from(rows).map((r) => r.dataset.path)).toEqual([
      "AGENTS.md",
      "CLAUDE.md",
      "docs/AGENTS.md",
    ]);
    expect(document.querySelector("[data-slot='rules-file-not-written']")).toBeNull();
    expect(screen.queryByRole("button", { name: "Write them now" })).toBeNull();

    await userEvent.click(within(rows[2]).getByRole("button"));
    expect(rows[2].querySelector("[data-slot='rules-file-section']")?.textContent).toContain(
      'select, not "click on"',
    );
  });

  it("marks a file kapi has not written yet and offers Write them now", () => {
    render(<RulesFilesView tabID="t1" rulesFiles={RULES_FILES_PARTLY_WRITTEN} />);
    const marked = document.querySelectorAll("[data-slot='rules-file-not-written']");
    expect(marked).toHaveLength(1);
    expect(marked[0].textContent).toBe("not written yet");
    expect(marked[0].closest<HTMLElement>("[data-slot='rules-file']")?.dataset.path).toBe(
      "docs/AGENTS.md",
    );
    expect(document.querySelector("[data-slot='rules-files-empty']")).toBeNull();
    expect(screen.getByRole("button", { name: "Write them now" })).toBeInTheDocument();
  });

  it("before any file is written, the empty state says where the rules go and writes them", async () => {
    const write = vi.spyOn(api, "writeRulesFiles").mockResolvedValue(WRITE_RULES_RESULT);
    render(<RulesFilesView tabID="t1" rulesFiles={RULES_FILES_NOT_WRITTEN} />);
    const empty = document.querySelector<HTMLElement>("[data-slot='rules-files-empty']")!;
    expect(within(empty).getByText("No rules file written yet")).toBeInTheDocument();
    expect(within(empty).getByText(HELP)).toBeInTheDocument();
    expect(document.querySelectorAll("[data-slot='rules-file-not-written']")).toHaveLength(3);
    expect(screen.getAllByRole("button", { name: "Write them now" })).toHaveLength(1);

    await userEvent.click(within(empty).getByRole("button", { name: "Write them now" }));
    expect(write).toHaveBeenCalledWith("t1");
    const wrote = await waitFor(() => {
      const el = document.querySelector("[data-slot='rules-files-wrote']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(Array.from(wrote.querySelectorAll("li")).map((li) => li.textContent)).toEqual([
      "AGENTS.md · created",
      "CLAUDE.md · created",
      "docs/AGENTS.md · created",
    ]);
  });

  it("says so when the context holds no rules", () => {
    render(<RulesFilesView tabID="t1" rulesFiles={[]} />);
    const none = document.querySelector<HTMLElement>("[data-slot='rules-files-none']")!;
    expect(within(none).getByText("No rules files")).toBeInTheDocument();
    expect(
      within(none).getByText("The project's context holds no rules yet, so kapi writes no file."),
    ).toBeInTheDocument();
  });
});

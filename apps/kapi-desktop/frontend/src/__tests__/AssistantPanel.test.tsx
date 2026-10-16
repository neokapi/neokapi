import { afterEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import { AssistantPanel } from "../components/AssistantPanel";
import { AssistantHosts, canConnect, orderHosts } from "../components/AssistantHosts";
import { RulesFilesView } from "../components/RulesFilesView";
import { api } from "../hooks/useApi";
import {
  CONNECT_CLAUDE_DESKTOP_RESULT,
  CONNECT_CURSOR_RESULT,
  CLAUDE_DESKTOP_CONFIG,
  HOSTS_ALL_CONNECTED,
  HOSTS_CLAUDE_DESKTOP_ABSENT,
  HOSTS_NONE_CONNECTED,
} from "../stories/fixtures/agentHosts";
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

const hostRow = (host: string) =>
  document.querySelector<HTMLElement>(`[data-slot='assistant-host'][data-host='${host}']`);
const hostOrder = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='assistant-host']")).map(
    (el) => el.dataset.host,
  );
const connectButton = (host: string) =>
  hostRow(host)?.querySelector<HTMLButtonElement>("[data-slot='assistant-host-connect']") ?? null;
const caption = (host: string) =>
  hostRow(host)?.querySelector("[data-slot='assistant-host-connected']")?.textContent?.trim() ??
  null;
const dot = (host: string) =>
  hostRow(host)?.querySelector<HTMLElement>("[data-slot='assistant-host-dot']") ?? null;

/** The brief's backend: the files the picker offers, and the one answer. */
function stubBrief() {
  vi.spyOn(api, "contextOptions").mockResolvedValue(BRIEF_FILES);
  return vi.spyOn(api, "agentContextAt").mockResolvedValue(BRIEF_VIEW);
}

describe("What your assistant reads", () => {
  it("shows the connections, then two tabs, opening on the rules files", () => {
    render(
      <AssistantPanel tabID="t1" hosts={HOSTS_ALL_CONNECTED} rulesFiles={RULES_FILES_WRITTEN} />,
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "What your assistant reads",
    );
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Connected assistants",
    ]);
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Rules files",
      "The brief for a file",
    ]);
    expect(screen.getByRole("tab", { name: "Rules files" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(document.querySelector("[data-slot='rules-files']")).not.toBeNull();
    expect(document.querySelector("[data-slot='assistant-brief']")).toBeNull();
  });

  it("names no command line and explains nothing under a heading", () => {
    render(
      <AssistantPanel tabID="t1" hosts={HOSTS_ALL_CONNECTED} rulesFiles={RULES_FILES_WRITTEN} />,
    );
    const panel = document.querySelector<HTMLElement>("[data-slot='assistant-panel']")!;
    expect(panel.textContent).not.toMatch(/The same as/);
    expect(panel.textContent).not.toContain(HELP);
  });

  it("switches to the brief for a file, with the picker and what it needs", async () => {
    stubBrief();
    render(
      <AssistantPanel tabID="t1" hosts={HOSTS_ALL_CONNECTED} rulesFiles={RULES_FILES_WRITTEN} />,
    );
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
        hosts={HOSTS_ALL_CONNECTED}
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
    const { rerender } = render(
      <AssistantPanel tabID="t1" hosts={HOSTS_ALL_CONNECTED} rulesFiles={RULES_FILES_WRITTEN} />,
    );
    expect(screen.getByRole("tab", { name: "Rules files" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    rerender(
      <AssistantPanel
        tabID="t1"
        path={BRIEF_PATH}
        hosts={HOSTS_ALL_CONNECTED}
        rulesFiles={RULES_FILES_WRITTEN}
      />,
    );
    expect(screen.getByRole("tab", { name: "The brief for a file" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await waitFor(() => expect(read).toHaveBeenCalledWith("t1", BRIEF_PATH, 0));
  });

  it("context-only: says connecting needs a checkout and shows no rows or tabs", () => {
    render(<AssistantPanel tabID="t1" contextOnly />);
    expect(
      document.querySelector("[data-slot='assistant-hosts-needs-checkout']")?.textContent,
    ).toBe("Connecting an assistant needs a checkout of this project on this Mac.");
    expect(document.querySelector("[data-slot='assistant-host']")).toBeNull();
    expect(screen.queryByRole("tab")).toBeNull();
    expect(document.querySelector("[data-slot='rules-files']")).toBeNull();
    expect(document.querySelector("[data-slot='assistant-brief']")).toBeNull();
  });
});

describe("connected assistants", () => {
  it("marks each connected assistant with a filled dot and Connected, and gives its row no button", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_ALL_CONNECTED} />);
    for (const host of ["claude-code", "cursor", "vscode", "codex", "claude-desktop"]) {
      expect(dot(host)?.className).toContain("bg-success");
      expect(caption(host)).toBe("Connected");
      expect(hostRow(host)?.querySelector("button")).toBeNull();
    }
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(screen.queryByRole("button", { name: "Connect" })).toBeNull();
    expect(screen.queryByText(/kapi init/)).toBeNull();
    expect(screen.queryByText(/not connected/)).toBeNull();
  });

  it("offers Connect on every assistant that is not connected, with an outline dot and no caption", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_NONE_CONNECTED} />);
    for (const host of ["claude-code", "cursor", "vscode", "codex", "claude-desktop"]) {
      expect(connectButton(host)).not.toBeNull();
      expect(caption(host)).toBeNull();
      expect(dot(host)?.className).not.toContain("bg-success");
    }
    expect(hostRow("cursor")).toHaveTextContent(/^Cursor\s*Connect$/);
  });

  it("lists assistants that are neither here nor connected last, muted, and Claude Desktop without Connect", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_CLAUDE_DESKTOP_ABSENT} />);
    expect(hostOrder()).toEqual(["claude-code", "cursor", "vscode", "codex", "claude-desktop"]);
    expect(hostRow("claude-code")?.dataset.absent).toBeUndefined();
    expect(hostRow("vscode")?.dataset.absent).toBe("true");
    expect(hostRow("claude-desktop")?.dataset.absent).toBe("true");
    expect(connectButton("claude-desktop")).toBeNull();
    expect(hostRow("claude-code")?.querySelector("button")).toBeNull();
    expect(connectButton("vscode")).not.toBeNull();
  });

  it("orders present assistants before absent ones", () => {
    const absentFirst = [HOSTS_CLAUDE_DESKTOP_ABSENT[4], HOSTS_CLAUDE_DESKTOP_ABSENT[0]];
    expect(orderHosts(absentFirst).map((h) => h.host)).toEqual(["claude-code", "claude-desktop"]);
    expect(canConnect(HOSTS_CLAUDE_DESKTOP_ABSENT[4])).toBe(false);
    expect(canConnect(HOSTS_NONE_CONNECTED[4])).toBe(true);
  });

  it("Connect writes the files, shows what it wrote, and reads the hosts again", async () => {
    const read = vi.spyOn(api, "agentHosts").mockResolvedValue(HOSTS_NONE_CONNECTED);
    const connect = vi.spyOn(api, "connectAgentHost").mockResolvedValue(CONNECT_CURSOR_RESULT);
    render(<AssistantHosts tabID="t1" />);
    await waitFor(() => expect(connectButton("cursor")).not.toBeNull());

    await userEvent.click(connectButton("cursor")!);
    expect(connect).toHaveBeenCalledWith("t1", "cursor");
    await waitFor(() =>
      expect(
        hostRow("cursor")?.querySelector("[data-slot='assistant-host-wrote']")?.textContent,
      ).toBe(".cursor/mcp.json · created"),
    );
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  });

  it("asks before writing into Claude Desktop's own configuration", async () => {
    const connect = vi
      .spyOn(api, "connectAgentHost")
      .mockResolvedValue(CONNECT_CLAUDE_DESKTOP_RESULT);
    render(<AssistantHosts tabID="t1" hosts={HOSTS_NONE_CONNECTED} />);

    await userEvent.click(connectButton("claude-desktop")!);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/^This writes an entry/).textContent).toBe(
      `This writes an entry into your own Claude Desktop configuration at ${CLAUDE_DESKTOP_CONFIG}.`,
    );
    expect(connect).not.toHaveBeenCalled();

    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(connect).not.toHaveBeenCalled();

    await userEvent.click(connectButton("claude-desktop")!);
    await userEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Connect" }),
    );
    expect(connect).toHaveBeenCalledWith("t1", "claude-desktop");
  });

  it("ends the question without a path when the backend sends none", async () => {
    const noPath = { ...HOSTS_NONE_CONNECTED[4], files: undefined };
    render(<AssistantHosts tabID="t1" hosts={[noPath]} />);
    await userEvent.click(connectButton("claude-desktop")!);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/^This writes an entry/).textContent).toBe(
      "This writes an entry into your own Claude Desktop configuration.",
    );
  });

  it("connects every other assistant without asking", async () => {
    const connect = vi.spyOn(api, "connectAgentHost").mockResolvedValue(CONNECT_CURSOR_RESULT);
    render(<AssistantHosts tabID="t1" hosts={HOSTS_NONE_CONNECTED} />);
    await userEvent.click(connectButton("cursor")!);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(connect).toHaveBeenCalledWith("t1", "cursor");
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

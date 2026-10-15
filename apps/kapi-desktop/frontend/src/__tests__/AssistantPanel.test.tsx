import { afterEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import { AssistantPanel } from "../components/AssistantPanel";
import { AssistantHosts, canConnect, orderHosts } from "../components/AssistantHosts";
import { RulesFilesView } from "../components/RulesFilesView";
import { api } from "../components/contextTypes";
import {
  CLAUDE_DESKTOP_CONFIG,
  CONNECT_CLAUDE_DESKTOP_RESULT,
  CONNECT_CURSOR_RESULT,
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

afterEach(() => {
  vi.restoreAllMocks();
});

const hostRow = (host: string) =>
  document.querySelector<HTMLElement>(`[data-slot='assistant-host'][data-host='${host}']`);
const hostState = (host: string) =>
  hostRow(host)?.querySelector("[data-slot='assistant-host-state']")?.textContent ?? "";
const hostOrder = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='assistant-host']")).map(
    (el) => el.dataset.host,
  );
const connectButton = (host: string) =>
  hostRow(host)?.querySelector<HTMLButtonElement>("[data-slot='assistant-host-connect']") ?? null;

describe("What your assistant reads", () => {
  it("shows the connections, the rules files and the brief for a file, in that order", () => {
    render(
      <AssistantPanel tabID="t1" hosts={HOSTS_ALL_CONNECTED} rulesFiles={RULES_FILES_WRITTEN} />,
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "What your assistant reads",
    );
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Connected assistants",
      "Rules files",
      "The brief for a file",
    ]);
    expect(document.querySelector("[data-slot='assistant-brief'] p")?.textContent).toBe(
      "For one file, the full answer an assistant gets, the same text kapi context <path> prints.",
    );
  });

  it("context-only: says connecting needs a checkout and shows no rows", () => {
    render(<AssistantPanel tabID="t1" contextOnly />);
    expect(
      document.querySelector("[data-slot='assistant-hosts-needs-checkout']")?.textContent,
    ).toBe("Connecting an assistant needs a checkout of this project on this Mac.");
    expect(document.querySelector("[data-slot='assistant-host']")).toBeNull();
    expect(document.querySelector("[data-slot='rules-files']")).toBeNull();
    expect(document.querySelector("[data-slot='assistant-brief']")).toBeNull();
  });
});

describe("connected assistants", () => {
  it("lists each connected assistant with the files that carry it", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_ALL_CONNECTED} />);
    expect(hostState("claude-code")).toBe(
      "Claude Code · connected (.mcp.json, .claude/skills/kapi)",
    );
    expect(hostState("cursor")).toBe("Cursor · connected (.cursor/mcp.json)");
    expect(hostState("claude-desktop")).toBe(
      `Claude Desktop · connected (${CLAUDE_DESKTOP_CONFIG})`,
    );
    expect(screen.queryByRole("button", { name: "Connect" })).toBeNull();
    expect(screen.getByText("kapi init")).toBeInTheDocument();
  });

  it("offers Connect on every assistant that is not connected", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_NONE_CONNECTED} />);
    expect(hostState("cursor")).toBe("Cursor · not connected");
    for (const host of ["claude-code", "cursor", "vscode", "codex", "claude-desktop"]) {
      expect(connectButton(host)).not.toBeNull();
    }
  });

  it("lists assistants that are neither here nor connected last, muted, and Claude Desktop without Connect", () => {
    render(<AssistantHosts tabID="t1" hosts={HOSTS_CLAUDE_DESKTOP_ABSENT} />);
    expect(hostOrder()).toEqual(["claude-code", "cursor", "vscode", "codex", "claude-desktop"]);
    expect(hostRow("claude-code")?.dataset.absent).toBeUndefined();
    expect(hostRow("vscode")?.dataset.absent).toBe("true");
    expect(hostRow("claude-desktop")?.dataset.absent).toBe("true");
    expect(connectButton("claude-desktop")).toBeNull();
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

  it("connects every other assistant without asking", async () => {
    const connect = vi.spyOn(api, "connectAgentHost").mockResolvedValue(CONNECT_CURSOR_RESULT);
    render(<AssistantHosts tabID="t1" hosts={HOSTS_NONE_CONNECTED} />);
    await userEvent.click(connectButton("cursor")!);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(connect).toHaveBeenCalledWith("t1", "cursor");
  });
});

describe("rules files", () => {
  it("says where kapi writes the rules and lists each file with its section", async () => {
    render(<RulesFilesView tabID="t1" rulesFiles={RULES_FILES_WRITTEN} />);
    expect(document.querySelector("[data-slot='rules-files'] p")?.textContent).toBe(
      "kapi writes the rules into AGENTS.md and CLAUDE.md, which an assistant reads as it starts.",
    );
    const rows = document.querySelectorAll<HTMLElement>("[data-slot='rules-file']");
    expect(Array.from(rows).map((r) => r.dataset.path)).toEqual([
      "AGENTS.md",
      "CLAUDE.md",
      "docs/AGENTS.md",
    ]);
    expect(document.querySelector("[data-slot='rules-file-not-written']")).toBeNull();
    expect(screen.queryByRole("button", { name: "Write them now" })).toBeNull();
    expect(screen.getByText("kapi context sync --files-only")).toBeInTheDocument();

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
    expect(screen.getByRole("button", { name: "Write them now" })).toBeInTheDocument();
  });

  it("Write them now writes the files and lists what it did", async () => {
    const write = vi.spyOn(api, "writeRulesFiles").mockResolvedValue(WRITE_RULES_RESULT);
    render(<RulesFilesView tabID="t1" rulesFiles={RULES_FILES_NOT_WRITTEN} />);
    expect(document.querySelectorAll("[data-slot='rules-file-not-written']")).toHaveLength(3);

    await userEvent.click(screen.getByRole("button", { name: "Write them now" }));
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
    expect(document.querySelector("[data-slot='rules-files-none']")?.textContent).toBe(
      "No rules files: the project's context holds no rules yet.",
    );
  });
});

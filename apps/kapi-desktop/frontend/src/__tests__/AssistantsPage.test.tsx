import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import { AssistantsPage, sessionCounts } from "../components/AssistantsPage";
import { api } from "../hooks/useApi";
import {
  ASSISTANTS_CHECK_PENDING,
  ASSISTANTS_CLAUDE_CONNECTED,
  ASSISTANTS_NO_CHECKOUT,
  ASSISTANTS_NONE,
  CLAUDE_DESKTOP_CONFIG,
  CONNECT_CLAUDE_DESKTOP_RESULT,
  CONNECT_CURSOR_RESULT,
  CONNECT_NOTHING_RESULT,
  PLUGIN_COMMANDS,
  session,
} from "../stories/fixtures/assistants";
import type { AssistantsView } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

const row = (host: string) =>
  document.querySelector<HTMLElement>(`[data-slot='assistant-host'][data-host='${host}']`);
const order = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='assistant-host']")).map(
    (el) => el.dataset.host,
  );
const connectButton = (host: string) =>
  row(host)?.querySelector<HTMLButtonElement>("[data-slot='assistant-host-connect']") ?? null;
const state = (host: string) =>
  row(host)?.querySelector("[data-slot='assistant-host-state']")?.textContent?.trim() ?? null;

function page(view: AssistantsView, props: Partial<Parameters<typeof AssistantsPage>[0]> = {}) {
  const onShowReads = vi.fn();
  const onOpenSession = vi.fn();
  render(
    <AssistantsPage
      tabID="t1"
      projectName="KapiMart"
      onShowReads={onShowReads}
      onOpenSession={onOpenSession}
      view={view}
      {...props}
    />,
  );
  return { onShowReads, onOpenSession };
}

describe("the Assistants page", () => {
  it("titles the page with the project and says what connecting does", () => {
    page(ASSISTANTS_NONE);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("KapiMart · Assistants");
    expect(document.querySelector("[data-slot='assistants-pitch']")?.textContent).toBe(
      "Checks every edit · follows your 4 rules · leaves suggestions here",
    );
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Assistants",
      "Sessions",
    ]);
  });

  it("lists one row per assistant in kapi connect's order, then Claude Desktop", () => {
    page(ASSISTANTS_NONE);
    expect(order()).toEqual([
      "claude-code",
      "cursor",
      "vscode",
      "codex",
      "agents",
      "claude-desktop",
    ]);
    for (const host of order()) {
      expect(state(host!)).toBe("Not connected");
      expect(connectButton(host!)).not.toBeNull();
    }
    expect(document.querySelector("[data-slot='assistant-host-pending']")).toBeNull();
  });

  it("says what a connected assistant gets, and gives its row no button", () => {
    page(ASSISTANTS_CLAUDE_CONNECTED);
    expect(state("claude-code")).toBe("Connected · rules + check on every edit");
    expect(row("claude-code")!.querySelector("button")).toBeNull();
    expect(state("cursor")).toBe("Not connected");
  });

  it("shows what an assistant reads from the page, before any assistant is connected", async () => {
    const { onShowReads } = page(ASSISTANTS_NONE);
    await userEvent.click(screen.getByRole("button", { name: "Show what it reads" }));
    expect(onShowReads).toHaveBeenCalledOnce();
  });

  it("names the two plugin commands under Claude Code while the check waits on them", () => {
    page(ASSISTANTS_CHECK_PENDING);
    expect(state("claude-code")).toBe("Connected · rules");
    const pending = row("claude-code")!.querySelector("[data-slot='assistant-host-pending']")!;
    expect(Array.from(pending.querySelectorAll("code")).map((c) => c.textContent)).toEqual(
      PLUGIN_COMMANDS,
    );
  });

  it("Connect calls kapi connect's binding for that assistant and lists what it wrote", async () => {
    const read = vi.spyOn(api, "assistantsView").mockResolvedValue(ASSISTANTS_NONE);
    const connect = vi.spyOn(api, "connectAssistant").mockResolvedValue(CONNECT_CURSOR_RESULT);
    page(ASSISTANTS_NONE, { view: undefined });
    await waitFor(() => expect(connectButton("cursor")).not.toBeNull());

    await userEvent.click(connectButton("cursor")!);
    expect(connect).toHaveBeenCalledWith("t1", "cursor");
    await waitFor(() =>
      expect(
        Array.from(row("cursor")!.querySelectorAll("[data-slot='assistant-host-wrote'] li")).map(
          (li) => li.textContent,
        ),
      ).toEqual([".cursor/mcp.json · created", "AGENTS.md · updated"]),
    );
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  });

  it("says nothing was written when every file was in place", async () => {
    vi.spyOn(api, "connectAssistant").mockResolvedValue(CONNECT_NOTHING_RESULT);
    page(ASSISTANTS_NONE);
    await userEvent.click(connectButton("cursor")!);
    await waitFor(() =>
      expect(row("cursor")!.querySelector("[data-slot='assistant-host-wrote']")?.textContent).toBe(
        "Nothing was written.",
      ),
    );
  });

  it("asks before writing into Claude Desktop's own configuration", async () => {
    const wiring = vi
      .spyOn(api, "connectAgentHost")
      .mockResolvedValue(CONNECT_CLAUDE_DESKTOP_RESULT);
    const connect = vi.spyOn(api, "connectAssistant");
    page(ASSISTANTS_NONE);

    await userEvent.click(connectButton("claude-desktop")!);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/^This writes an entry/).textContent).toBe(
      `This writes an entry into your own Claude Desktop configuration at ${CLAUDE_DESKTOP_CONFIG}.`,
    );
    expect(wiring).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole("button", { name: "Connect" }));
    expect(wiring).toHaveBeenCalledWith("t1", "claude-desktop");
    expect(connect).not.toHaveBeenCalled();
  });

  it("offers no Connect for an assistant that cannot be connected here", () => {
    page({
      ...ASSISTANTS_NONE,
      hosts: ASSISTANTS_NONE.hosts.map((h) =>
        h.host === "claude-desktop" ? { ...h, available: false } : h,
      ),
    });
    expect(connectButton("claude-desktop")).toBeNull();
    expect(state("claude-desktop")).toBe("Not connected");
  });

  it("context-only: says connecting needs a checkout, and keeps the sessions", () => {
    page(ASSISTANTS_NO_CHECKOUT);
    expect(document.querySelector("[data-slot='assistants-needs-checkout']")?.textContent).toBe(
      "Connecting an assistant needs a checkout of this project on this Mac.",
    );
    expect(document.querySelector("[data-slot='assistant-host']")).toBeNull();
    expect(document.querySelector("[data-slot='assistants-pitch']")).toBeNull();
    expect(screen.queryByRole("button", { name: "Show what it reads" })).toBeNull();
    expect(document.querySelectorAll("[data-slot='assistant-session']")).toHaveLength(2);
  });
});

describe("sessions", () => {
  it("lists each session with its mark, who worked and its counts", () => {
    page(ASSISTANTS_CLAUDE_CONNECTED);
    const rows = Array.from(
      document.querySelectorAll<HTMLElement>("[data-slot='assistant-session']"),
    );
    expect(rows.map((r) => r.dataset.session)).toEqual([
      "session:sess-1",
      "actor:person/:2026-10-28",
    ]);
    expect(rows[0]).toHaveTextContent(/claude/);
    expect(
      rows.map((r) => r.querySelector("[data-slot='assistant-session-counts']")?.textContent),
    ).toEqual(["2 suggestions · 1 correction", "2 kept"]);
    expect(rows[0].querySelector("[data-slot='assistant-session-awaiting']")).not.toBeNull();
    expect(rows[1].querySelector("[data-slot='assistant-session-awaiting']")).toBeNull();
    expect(rows[1]).toHaveTextContent(/You/);
  });

  it("opens the Sessions view on the session chosen, or on the newest", async () => {
    const { onOpenSession } = page(ASSISTANTS_CLAUDE_CONNECTED);
    await userEvent.click(document.querySelector<HTMLElement>("[data-session='session:sess-1']")!);
    expect(onOpenSession).toHaveBeenLastCalledWith("session:sess-1");
    await userEvent.click(screen.getByRole("button", { name: "All sessions" }));
    expect(onOpenSession).toHaveBeenLastCalledWith();
  });

  it("shows the empty state when nothing is recorded", () => {
    page(ASSISTANTS_NONE);
    expect(screen.getByText("No sessions yet")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "All sessions" })).toBeNull();
  });

  it("counts at most three facts, and the operations when none applies", () => {
    expect(
      sessionCounts(session({ id: "s", suggestions: 1, corrections: 2, kept: 3, dropped: 4 })),
    ).toBe("1 suggestion · 2 corrections · 3 kept");
    expect(sessionCounts(session({ id: "s", operations: 2 }))).toBe("2 operations");
  });
});

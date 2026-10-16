import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";

// The explorer mounts the shared package against the Wails backend, which no
// test runtime provides; the hub's job is routing, so the panes stand in.
vi.mock("../components/ContextExplorerView", () => ({
  ContextExplorerView: ({ tabID, pin }: { tabID: string; pin?: { path?: string } }) => (
    <div>
      explorer for {tabID}
      {pin?.path && <span data-testid="explorer-pinned-path">{pin.path}</span>}
    </div>
  ),
}));
vi.mock("../components/VoicePage", () => ({
  VoicePage: ({ tabID }: { tabID: string }) => <div>voice for {tabID}</div>,
}));
vi.mock("../components/TermsPage", () => ({
  TermsPage: ({ tabID }: { tabID: string }) => <div>terms for {tabID}</div>,
}));
vi.mock("../components/MemoriesPage", () => ({
  MemoriesPage: ({ tabID, onOpenUnit }: { tabID: string; onOpenUnit?: (p: string) => void }) => (
    <div>
      memory for {tabID}
      <button onClick={() => onOpenUnit?.("web/en/index.md")}>open unit</button>
    </div>
  ),
}));

vi.mock("../components/ContextDigestPanel", () => ({
  ContextDigestPanel: ({
    tabID,
    onOpenFile,
    onConnectAssistant,
  }: {
    tabID: string;
    onOpenFile?: (p: string) => void;
    onConnectAssistant?: () => void;
  }) => (
    <div>
      digest for {tabID}
      <button onClick={() => onOpenFile?.("docs/billing.md")}>open file</button>
      {onConnectAssistant && <button onClick={onConnectAssistant}>connect</button>}
    </div>
  ),
}));
// The two panels have their own tests; here they stand in, saying what the
// hub handed them.
vi.mock("../components/AssistantPanel", () => ({
  AssistantPanel: ({ tabID, contextOnly }: { tabID: string; contextOnly?: boolean }) => (
    <div>
      assistant for {tabID}
      {contextOnly ? " without a checkout" : ""}
    </div>
  ),
}));
vi.mock("../components/SharePanel", () => ({
  SharePanel: ({ tabID, contextOnly }: { tabID: string; contextOnly?: boolean }) => (
    <div>
      share for {tabID}
      {contextOnly ? " without a checkout" : ""}
    </div>
  ),
}));
vi.mock("../components/ContextFeedPanel", () => ({
  ContextFeedPanel: ({ projectKey }: { projectKey?: string }) => <div>feed for {projectKey}</div>,
}));

import { ContextHub, contextSectionOf } from "../components/ContextHub";
import { api } from "../hooks/useApi";
import { CONTEXT_DIGEST } from "../stories/fixtures/contextDigest";

const railLabels = () =>
  Array.from(
    screen.getByRole("navigation", { name: "Context sections" }).querySelectorAll("button"),
  ).map((b) => b.textContent);

describe("ContextHub", () => {
  it("opens on What's new for the project", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    expect(screen.getByText("digest for t1")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "What's new for Northsea" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "What's new" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("names the sections in the order a person meets them", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" hasTargetLanguages />);
    expect(railLabels()).toEqual([
      "What's new",
      "Ask",
      "Assistant",
      "Voice",
      "Terms",
      "Content Memory",
      "History",
      "Share",
    ]);
  });

  it("keeps what a tab with no checkout can read, and drops what reads files", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    expect(railLabels()).toEqual([
      "What's new",
      "Assistant",
      "Terms",
      "Content Memory",
      "History",
      "Share",
    ]);
    await userEvent.click(screen.getByRole("button", { name: "Share" }));
    expect(screen.getByText("share for t1 without a checkout")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Assistant" }));
    expect(screen.getByText("assistant for t1 without a checkout")).toBeInTheDocument();
  });

  it("opens Share on the project's ladder", () => {
    render(<ContextHub tabID="t3" projectName="Northsea" section="share" />);
    expect(screen.getByText("share for t3")).toBeInTheDocument();
  });

  it("names the command each section does the work of, once, at the right of the nav", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    const commands = () =>
      Array.from(document.querySelectorAll("[data-slot='context-section-command']"));
    const nav = screen.getByRole("navigation", { name: "Context sections" });
    expect(commands()).toHaveLength(1);
    expect(nav).toContainElement(commands()[0] as HTMLElement);
    expect(commands()[0].textContent).toBe("kapi context review");
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    expect(commands()[0].textContent).toBe("kapi context log");
    await userEvent.click(screen.getByRole("button", { name: "Share" }));
    expect(commands()[0].textContent).toBe("kapi context sync");
    await userEvent.click(screen.getByRole("button", { name: "Assistant" }));
    expect(commands()[0].textContent).toBe("kapi init");
    await userEvent.click(screen.getByRole("button", { name: "Voice" }));
    expect(commands()).toHaveLength(0);
    vi.restoreAllMocks();
  });

  it("carries no caption under the What's new and History titles", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    const title = screen.getByRole("heading", { name: "What's new for Northsea" });
    expect(title.parentElement?.querySelector("p")).toBeNull();
    expect(document.body.textContent).not.toContain("The same as");
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    expect(
      screen.getByRole("heading", { name: "History" }).parentElement?.querySelector("p"),
    ).toBeNull();
    expect(document.body.textContent).not.toContain("The same as");
    vi.restoreAllMocks();
  });

  it("goes to the Assistant from the empty digest's link", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    await userEvent.click(screen.getByRole("button", { name: "connect" }));
    expect(screen.getByText("assistant for t1")).toBeInTheDocument();
  });

  it("lists the project's own history", async () => {
    const digest = vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" section="history" />);
    expect(await screen.findByText(`feed for ${CONTEXT_DIGEST.project}`)).toBeInTheDocument();
    expect(digest).toHaveBeenCalledWith("t1", "");
    digest.mockRestore();
  });

  it("routes a view to the section it names", () => {
    expect(contextSectionOf("context:agent")).toBe("agent");
    expect(contextSectionOf("context:history")).toBe("history");
    expect(contextSectionOf("context:share")).toBe("share");
    expect(contextSectionOf("termbases")).toBe("terms");
    expect(contextSectionOf("memories")).toBe("memory");
    expect(contextSectionOf("context")).toBeUndefined();
    expect(contextSectionOf("context:nowhere")).toBeUndefined();
  });

  it("opens the explorer at a file the digest names", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    await userEvent.click(screen.getByRole("button", { name: "open file" }));
    expect(screen.getByText("explorer for t1")).toBeInTheDocument();
    expect(screen.getByTestId("explorer-pinned-path")).toHaveTextContent("docs/billing.md");
  });

  it("moves to Voice and marks it current", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    await userEvent.click(screen.getByRole("button", { name: "Voice" }));
    expect(screen.getByText("voice for t1")).toBeInTheDocument();
    expect(screen.queryByText("digest for t1")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Voice" })).toHaveAttribute("aria-current", "page");
  });

  it("honours the section it is opened on", () => {
    render(<ContextHub tabID="t2" projectName="Northsea" section="voice" />);
    expect(screen.getByText("voice for t2")).toBeInTheDocument();
  });

  it("files the stores under Context", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" hasTargetLanguages />);
    await userEvent.click(screen.getByRole("button", { name: "Terms" }));
    expect(screen.getByText("terms for t1")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Content Memory" }));
    expect(screen.getByText("memory for t1")).toBeInTheDocument();
  });

  it("keeps Content Memory quiet until the project declares targets", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" hasTargetLanguages={false} />);
    expect(screen.getByRole("button", { name: "Terms" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
  });

  it("opens the explorer standing at a unit the memory browser names", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" hasTargetLanguages />);
    await userEvent.click(screen.getByRole("button", { name: "Content Memory" }));
    await userEvent.click(screen.getByRole("button", { name: "open unit" }));

    expect(screen.getByText("explorer for t1")).toBeInTheDocument();
    expect(screen.getByTestId("explorer-pinned-path")).toHaveTextContent("web/en/index.md");
  });

  it("falls back to the digest when the opened section is gated away", () => {
    render(
      <ContextHub tabID="t1" projectName="Northsea" section="memory" hasTargetLanguages={false} />,
    );
    expect(screen.getByText("digest for t1")).toBeInTheDocument();
    expect(screen.queryByText("memory for t1")).not.toBeInTheDocument();
  });
});

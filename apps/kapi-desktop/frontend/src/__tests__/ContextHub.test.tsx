import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";

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
vi.mock("../components/ContextAtlasView", () => ({
  ContextAtlasView: ({
    tabID,
    onOpenEvidence,
  }: {
    tabID: string;
    onOpenEvidence?: (p: string) => void;
  }) => (
    <div>
      atlas for {tabID}
      {onOpenEvidence && (
        <button onClick={() => onOpenEvidence("src/en/store-ui.json")}>open evidence</button>
      )}
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
// The panels have their own tests; here they stand in, saying what the hub
// handed them.
vi.mock("../components/AssistantsPage", () => ({
  AssistantsPage: ({
    tabID,
    projectName,
    onShowReads,
    onOpenSession,
  }: {
    tabID: string;
    projectName: string;
    onShowReads: () => void;
    onOpenSession: (id?: string) => void;
  }) => (
    <div>
      assistants for {tabID} in {projectName}
      <button onClick={onShowReads}>show reads</button>
      <button onClick={() => onOpenSession("session:sess-1")}>open session</button>
    </div>
  ),
}));
vi.mock("../components/AssistantPanel", () => ({
  AssistantPanel: ({
    tabID,
    path,
    onBack,
  }: {
    tabID: string;
    path?: string;
    onBack?: () => void;
  }) => (
    <div>
      reads for {tabID}
      {path ? ` at ${path}` : ""}
      {onBack && <button onClick={onBack}>back to assistants</button>}
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
  ContextFeedPanel: ({
    projectKey,
    defaultSelected,
  }: {
    projectKey?: string;
    defaultSelected?: string;
  }) => (
    <div>
      feed for {projectKey}
      {defaultSelected ? ` on ${defaultSelected}` : ""}
    </div>
  ),
}));
vi.mock("../components/DecidePage", () => ({
  DecidePage: ({
    tabID,
    onOpenFile,
    onOpenMap,
  }: {
    tabID: string;
    onOpenFile?: (p: string) => void;
    onOpenMap?: () => void;
  }) => (
    <div>
      decide for {tabID}
      <button onClick={() => onOpenFile?.("docs/billing.md")}>open file</button>
      {onOpenMap && <button onClick={onOpenMap}>open the map</button>}
    </div>
  ),
}));

import { ContextHub, contextSectionOf } from "../components/ContextHub";
import { api } from "../hooks/useApi";
import { CONTEXT_DIGEST } from "../stories/fixtures/contextDigest";

const railLabels = () =>
  Array.from(
    screen.getByRole("navigation", { name: "Context sections" }).querySelectorAll("button"),
  ).map((b) => b.textContent);

describe("ContextHub", () => {
  it("opens on Decide", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    expect(screen.getByText("decide for t1")).toBeInTheDocument();
    expect(screen.queryByText("digest for t1")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Decide" })).toHaveAttribute("aria-current", "page");
  });

  it("opens the digest under Decide on a tab with no checkout", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    expect(screen.getByText("digest for t1")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Decide" })).toBeInTheDocument();
    expect(screen.queryByText("decide for t1")).not.toBeInTheDocument();
  });

  it("goes to the Map from Decide", async () => {
    const onNavigate = vi.fn();
    render(<ContextHub tabID="t1" projectName="Northsea" onNavigate={onNavigate} />);
    await userEvent.click(screen.getByRole("button", { name: "open the map" }));
    expect(onNavigate).toHaveBeenCalledWith("map");
  });

  it("names the sections in the order a person meets them", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" hasTargetLanguages />);
    expect(railLabels()).toEqual([
      "Decide",
      "Explore",
      "Ask",
      "Assistants",
      "Voice",
      "Terms",
      "Content Memory",
      "Share",
    ]);
  });

  it("keeps what a tab with no checkout can read, and drops what reads files", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    expect(railLabels()).toEqual([
      "Decide",
      "Explore",
      "Assistants",
      "Terms",
      "Content Memory",
      "Share",
    ]);
    await userEvent.click(screen.getByRole("button", { name: "Share" }));
    expect(screen.getByText("share for t1 without a checkout")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Assistants" }));
    expect(screen.getByText("assistants for t1 in Northsea")).toBeInTheDocument();
  });

  it("opens what an assistant reads from the Assistants page, and comes back", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" section="agent" />);
    await userEvent.click(screen.getByRole("button", { name: "show reads" }));
    expect(screen.getByText("reads for t1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Assistants" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await userEvent.click(screen.getByRole("button", { name: "back to assistants" }));
    expect(screen.getByText("assistants for t1 in Northsea")).toBeInTheDocument();
  });

  it("opens what an assistant reads at a file it was handed", () => {
    render(
      <ContextHub tabID="t1" projectName="Northsea" section="agent" pin={{ path: "docs/a.md" }} />,
    );
    expect(screen.getByText("reads for t1 at docs/a.md")).toBeInTheDocument();
  });

  it("opens Sessions on the session the Assistants page names, under Assistants on the rail", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" section="agent" />);
    await userEvent.click(screen.getByRole("button", { name: "open session" }));
    expect(
      await screen.findByText(`feed for ${CONTEXT_DIGEST.project} on session:sess-1`),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Sessions" })).toBeInTheDocument();
    const rail = screen.getByRole("navigation", { name: "Context sections" });
    expect(within(rail).getByRole("button", { name: "Assistants" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    // The back button and the rail entry share the name; the back button sits in the page.
    const back = document.querySelector<HTMLElement>("[data-slot='sessions-back']")!;
    await userEvent.click(back);
    expect(screen.getByText("assistants for t1 in Northsea")).toBeInTheDocument();
    vi.restoreAllMocks();
  });

  it("opens Explore, and its evidence in Ask", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" section="atlas" />);
    expect(screen.getByText("atlas for t1")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "open evidence" }));
    expect(screen.getByTestId("explorer-pinned-path")).toHaveTextContent("src/en/store-ui.json");
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
    expect(commands()[0].textContent).toBe("kapi rules");
    await userEvent.click(screen.getByRole("button", { name: "Share" }));
    expect(commands()[0].textContent).toBe("kapi context sync");
    await userEvent.click(screen.getByRole("button", { name: "Assistants" }));
    expect(commands()[0].textContent).toBe("kapi connect");
    await userEvent.click(screen.getByRole("button", { name: "open session" }));
    expect(commands()[0].textContent).toBe("kapi context log");
    await userEvent.click(screen.getByRole("button", { name: "Ask" }));
    expect(commands()[0].textContent).toBe("kapi context <path>");
    await userEvent.click(screen.getByRole("button", { name: "Voice" }));
    expect(commands()[0].textContent).toBe("kapi voice show");
    // Terms and Content Memory search and look up: no one command is the page.
    await userEvent.click(screen.getByRole("button", { name: "Terms" }));
    expect(commands()).toHaveLength(0);
    vi.restoreAllMocks();
  });

  it("names the review as Decide's command on a tab with no checkout, which reads the digest", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    expect(document.querySelector("[data-slot='context-section-command']")?.textContent).toBe(
      "kapi context review",
    );
  });

  it("carries no caption under the Decide and Sessions titles", async () => {
    vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    const title = screen.getByRole("heading", { name: "Decide" });
    expect(title.parentElement?.querySelector("p")).toBeNull();
    expect(document.body.textContent).not.toContain("The same as");
    await userEvent.click(screen.getByRole("button", { name: "Assistants" }));
    await userEvent.click(screen.getByRole("button", { name: "open session" }));
    expect(
      screen.getByRole("heading", { name: "Sessions" }).parentElement?.querySelector("p"),
    ).toBeNull();
    expect(document.body.textContent).not.toContain("The same as");
    vi.restoreAllMocks();
  });

  it("goes to the Assistants from the empty digest's link", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" contextOnly />);
    await userEvent.click(screen.getByRole("button", { name: "connect" }));
    expect(screen.getByText("assistants for t1 in Northsea")).toBeInTheDocument();
  });

  it("lists the project's own history", async () => {
    const digest = vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    const mark = vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<ContextHub tabID="t1" projectName="Northsea" section="history" />);
    expect(await screen.findByText(`feed for ${CONTEXT_DIGEST.project}`)).toBeInTheDocument();
    expect(digest).toHaveBeenCalledWith("t1", "");
    // Sessions lists what is new, so opening it moves the home's marker.
    expect(mark).toHaveBeenCalledWith("t1");
    digest.mockRestore();
    mark.mockRestore();
  });

  it("gives Sessions the full height, so its list and detail scroll on their own", async () => {
    const digest = vi.spyOn(api, "projectContextDigest").mockResolvedValue(CONTEXT_DIGEST);
    render(<ContextHub tabID="t1" projectName="Northsea" section="history" />);
    const feed = await screen.findByText(`feed for ${CONTEXT_DIGEST.project}`);
    // No scrolling column around the panel: it fills the section's height.
    expect(feed.closest("[data-slot='scroll-area']")).toBeNull();
    expect(feed.closest(".max-w-3xl")).toBeNull();
    expect(feed.parentElement?.className).toContain("flex-1");
    expect(screen.getByRole("heading", { name: "Sessions" })).toBeInTheDocument();
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

  it("opens the explorer at a file Decide names", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    await userEvent.click(screen.getByRole("button", { name: "open file" }));
    expect(screen.getByText("explorer for t1")).toBeInTheDocument();
    expect(screen.getByTestId("explorer-pinned-path")).toHaveTextContent("docs/billing.md");
  });

  it("moves to Voice and marks it current", async () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    await userEvent.click(screen.getByRole("button", { name: "Voice" }));
    expect(screen.getByText("voice for t1")).toBeInTheDocument();
    expect(screen.queryByText("decide for t1")).not.toBeInTheDocument();
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

  it("falls back to Decide when the opened section is gated away", () => {
    render(
      <ContextHub tabID="t1" projectName="Northsea" section="memory" hasTargetLanguages={false} />,
    );
    expect(screen.getByText("decide for t1")).toBeInTheDocument();
    expect(screen.queryByText("memory for t1")).not.toBeInTheDocument();
  });
});

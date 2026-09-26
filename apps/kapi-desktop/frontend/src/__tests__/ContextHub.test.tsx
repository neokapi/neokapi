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
  }: {
    tabID: string;
    onOpenFile?: (p: string) => void;
  }) => (
    <div>
      digest for {tabID}
      <button onClick={() => onOpenFile?.("docs/billing.md")}>open file</button>
    </div>
  ),
}));

import { ContextHub } from "../components/ContextHub";

describe("ContextHub", () => {
  it("opens on what kapi learned", () => {
    render(<ContextHub tabID="t1" projectName="Northsea" />);
    expect(screen.getByText("digest for t1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Learned" })).toHaveAttribute("aria-current", "page");
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

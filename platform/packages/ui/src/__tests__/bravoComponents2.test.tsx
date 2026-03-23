import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BravoColdStart } from "../components/bravo/BravoColdStart";
import { BravoModeSelector } from "../components/bravo/BravoModeSelector";
import { BravoPanelTrigger } from "../components/bravo/BravoPanelTrigger";
import { BravoConversationList } from "../components/bravo/BravoConversationList";
import { BravoStepUpCard } from "../components/bravo/BravoStepUpCard";
import type { BravoConversation } from "../types/api";

// ---------------------------------------------------------------------------
// BravoColdStart
// ---------------------------------------------------------------------------

describe("BravoColdStart", () => {
  it("renders 'Waking up @bravo...' text", () => {
    render(<BravoColdStart />);
    expect(screen.getByText("Waking up @bravo...")).toBeDefined();
  });

  it("renders the timing description about 15-30 seconds", () => {
    render(<BravoColdStart />);
    expect(
      screen.getByText(/15-30 seconds/),
    ).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// BravoModeSelector
// ---------------------------------------------------------------------------

describe("BravoModeSelector", () => {
  it("renders all three mode labels", () => {
    render(<BravoModeSelector mode="ask" onChange={vi.fn()} />);
    expect(screen.getByText("Ask")).toBeDefined();
    expect(screen.getByText("Co-worker")).toBeDefined();
    expect(screen.getByText("Voice")).toBeDefined();
  });

  it("marks the current mode as aria-checked", () => {
    render(<BravoModeSelector mode="coworker" onChange={vi.fn()} />);
    const coworkerRadio = screen.getByRole("radio", { name: "Co-worker" });
    expect(coworkerRadio.getAttribute("aria-checked")).toBe("true");

    const askRadio = screen.getByRole("radio", { name: "Ask" });
    expect(askRadio.getAttribute("aria-checked")).toBe("false");

    const voiceRadio = screen.getByRole("radio", { name: "Voice" });
    expect(voiceRadio.getAttribute("aria-checked")).toBe("false");
  });

  it("calls onChange when a different mode is clicked", () => {
    const onChange = vi.fn();
    render(<BravoModeSelector mode="ask" onChange={onChange} />);
    fireEvent.click(screen.getByText("Co-worker"));
    expect(onChange).toHaveBeenCalledWith("coworker");
  });
});

// ---------------------------------------------------------------------------
// BravoPanelTrigger
// ---------------------------------------------------------------------------

describe("BravoPanelTrigger", () => {
  it("renders with accessible label", () => {
    render(<BravoPanelTrigger onClick={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Toggle @bravo assistant" })).toBeDefined();
  });

  it("calls onClick when clicked", async () => {
    const onClick = vi.fn();
    render(<BravoPanelTrigger onClick={onClick} />);
    await userEvent.setup().click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("shows unread indicator when hasUnread is true", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} hasUnread />);
    // The unread indicator is a span with animate-pulse class inside the button
    const indicator = container.querySelector(".animate-pulse");
    expect(indicator).not.toBeNull();
  });

  it("hides unread indicator when hasUnread is false", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} hasUnread={false} />);
    const indicator = container.querySelector(".animate-pulse");
    expect(indicator).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BravoConversationList
// ---------------------------------------------------------------------------

const makeConv = (overrides: Partial<BravoConversation> = {}): BravoConversation => ({
  id: "conv-1",
  workspace_id: "ws-1",
  user_id: "user-1",
  project_id: "proj-1",
  title: "Test conversation",
  status: "active",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
});

describe("BravoConversationList", () => {
  it("renders 'New conversation' button", () => {
    render(<BravoConversationList conversations={[]} />);
    expect(screen.getByText("New conversation")).toBeDefined();
  });

  it("renders conversation titles", () => {
    const convs = [makeConv({ id: "1", title: "Alpha" }), makeConv({ id: "2", title: "Beta" })];
    render(<BravoConversationList conversations={convs} />);
    expect(screen.getByText("Alpha")).toBeDefined();
    expect(screen.getByText("Beta")).toBeDefined();
  });

  it("shows 'No conversations yet' when empty and not loading", () => {
    render(<BravoConversationList conversations={[]} loading={false} />);
    expect(screen.getByText("No conversations yet")).toBeDefined();
  });

  it("shows 'Loading...' when loading with no conversations", () => {
    render(<BravoConversationList conversations={[]} loading />);
    expect(screen.getByText("Loading...")).toBeDefined();
  });

  it("shows 'Untitled' for conversations without title", () => {
    render(<BravoConversationList conversations={[makeConv({ title: "" })]} />);
    expect(screen.getByText("Untitled")).toBeDefined();
  });

  it("calls onNew when new conversation button clicked", async () => {
    const onNew = vi.fn();
    render(<BravoConversationList conversations={[]} onNew={onNew} />);
    await userEvent.setup().click(screen.getByText("New conversation"));
    expect(onNew).toHaveBeenCalledOnce();
  });

  it("calls onSelect when a conversation is clicked", async () => {
    const onSelect = vi.fn();
    const conv = makeConv({ title: "My Conv" });
    render(<BravoConversationList conversations={[conv]} onSelect={onSelect} />);
    await userEvent.setup().click(screen.getByText("My Conv"));
    expect(onSelect).toHaveBeenCalledWith(conv);
  });

  it("calls onDelete when delete button clicked", async () => {
    const onDelete = vi.fn();
    const conv = makeConv({ title: "Delete Me" });
    render(<BravoConversationList conversations={[conv]} onDelete={onDelete} />);
    const deleteBtn = screen.getByLabelText("Delete conversation");
    await userEvent.setup().click(deleteBtn);
    expect(onDelete).toHaveBeenCalledWith(conv);
  });
});

// ---------------------------------------------------------------------------
// BravoStepUpCard
// ---------------------------------------------------------------------------

describe("BravoStepUpCard", () => {
  it("shows required mode name and current mode name", () => {
    render(
      <BravoStepUpCard
        currentMode="ask"
        requiredMode="coworker"
        action="Running flows"
        onSwitchMode={vi.fn()}
        onDismiss={vi.fn()}
      />,
    );
    expect(screen.getByText(/requires Co-worker mode/)).toBeDefined();
    expect(screen.getByText(/currently in Ask mode/)).toBeDefined();
  });

  it("calls onSwitchMode with required mode when switch button clicked", () => {
    const onSwitchMode = vi.fn();
    render(
      <BravoStepUpCard
        currentMode="ask"
        requiredMode="coworker"
        action="Running flows"
        onSwitchMode={onSwitchMode}
        onDismiss={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByText("Switch to Co-worker"));
    expect(onSwitchMode).toHaveBeenCalledWith("coworker");
  });

  it("calls onDismiss when stay button clicked", () => {
    const onDismiss = vi.fn();
    render(
      <BravoStepUpCard
        currentMode="ask"
        requiredMode="coworker"
        action="Running flows"
        onSwitchMode={vi.fn()}
        onDismiss={onDismiss}
      />,
    );
    fireEvent.click(screen.getByText("Stay in Ask"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

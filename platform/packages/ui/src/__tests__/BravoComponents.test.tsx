import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent } from "@testing-library/react";
import { BravoComposer } from "../components/bravo/BravoComposer";
import { BravoPanelTrigger } from "../components/bravo/BravoPanelTrigger";
import { BravoConversationList } from "../components/bravo/BravoConversationList";
import { BravoApprovalCard } from "../components/bravo/BravoApprovalCard";
import { BravoThread } from "../components/bravo/BravoThread";
import type { BravoConversation, BravoMessage } from "../types/api";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const conv1: BravoConversation = {
  id: "c1",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "First chat",
  status: "active",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const conv2: BravoConversation = {
  id: "c2",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "Second chat",
  status: "completed",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const userMsg: BravoMessage = {
  id: "m1",
  conversation_id: "c1",
  role: "user",
  content: "Hello bravo",
  created_at: new Date().toISOString(),
};

const assistantMsg: BravoMessage = {
  id: "m2",
  conversation_id: "c1",
  role: "assistant",
  content: "Hi! How can I help?",
  input_tokens: 10,
  output_tokens: 20,
  created_at: new Date().toISOString(),
};

const msgWithToolCall: BravoMessage = {
  id: "m3",
  conversation_id: "c1",
  role: "assistant",
  content: "Running a tool...",
  tool_calls: [
    {
      id: "tc1",
      message_id: "m3",
      tool_name: "translate",
      input: { text: "hello" },
      status: "needs_approval",
      duration: 0,
    },
  ],
  created_at: new Date().toISOString(),
};

// ---------------------------------------------------------------------------
// BravoComposer
// ---------------------------------------------------------------------------

describe("BravoComposer", () => {
  it("renders with placeholder text", () => {
    render(<BravoComposer onSend={vi.fn()} />);
    const textarea = screen.getByPlaceholderText("Message @bravo...");
    expect(textarea).toBeDefined();
  });

  it("renders with custom placeholder", () => {
    render(<BravoComposer onSend={vi.fn()} placeholder="Ask anything..." />);
    expect(screen.getByPlaceholderText("Ask anything...")).toBeDefined();
  });

  it("calls onSend with trimmed value on button click", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} />);

    const textarea = screen.getByPlaceholderText("Message @bravo...");
    fireEvent.change(textarea, { target: { value: "  hello world  " } });
    fireEvent.click(screen.getByText("Send"));

    expect(onSend).toHaveBeenCalledWith("hello world");
  });

  it("calls onSend on Enter key (without Shift)", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} />);

    const textarea = screen.getByPlaceholderText("Message @bravo...");
    fireEvent.change(textarea, { target: { value: "test" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    expect(onSend).toHaveBeenCalledWith("test");
  });

  it("does not call onSend on Shift+Enter", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} />);

    const textarea = screen.getByPlaceholderText("Message @bravo...");
    fireEvent.change(textarea, { target: { value: "test" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("does not send when disabled", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} disabled />);

    const textarea = screen.getByPlaceholderText("Message @bravo...");
    fireEvent.change(textarea, { target: { value: "test" } });
    fireEvent.click(screen.getByText("Send"));

    expect(onSend).not.toHaveBeenCalled();
  });

  it("does not send empty or whitespace-only input", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} />);

    const textarea = screen.getByPlaceholderText("Message @bravo...");
    fireEvent.change(textarea, { target: { value: "   " } });
    fireEvent.click(screen.getByText("Send"));

    expect(onSend).not.toHaveBeenCalled();
  });

  it("clears textarea after sending", () => {
    const onSend = vi.fn();
    render(<BravoComposer onSend={onSend} />);

    const textarea = screen.getByPlaceholderText("Message @bravo...") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "hello" } });
    fireEvent.click(screen.getByText("Send"));

    expect(textarea.value).toBe("");
  });
});

// ---------------------------------------------------------------------------
// BravoPanelTrigger
// ---------------------------------------------------------------------------

describe("BravoPanelTrigger", () => {
  it("renders @bravo label", () => {
    render(<BravoPanelTrigger onClick={vi.fn()} />);
    expect(screen.getByText("@bravo")).toBeDefined();
  });

  it("calls onClick when clicked", () => {
    const onClick = vi.fn();
    render(<BravoPanelTrigger onClick={onClick} />);
    fireEvent.click(screen.getByText("@bravo"));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("shows unread indicator when hasUnread is true", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} hasUnread />);
    const dot = container.querySelector(".rounded-full.bg-primary");
    expect(dot).not.toBeNull();
  });

  it("does not show unread indicator by default", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} />);
    const dot = container.querySelector(".rounded-full.bg-primary");
    expect(dot).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BravoConversationList
// ---------------------------------------------------------------------------

describe("BravoConversationList", () => {
  it("renders new conversation button", () => {
    render(<BravoConversationList conversations={[]} />);
    expect(screen.getByText("New conversation")).toBeDefined();
  });

  it("shows empty state when no conversations and not loading", () => {
    render(<BravoConversationList conversations={[]} />);
    expect(screen.getByText("No conversations yet")).toBeDefined();
  });

  it("shows loading state when loading with no conversations", () => {
    render(<BravoConversationList conversations={[]} loading />);
    expect(screen.getByText("Loading...")).toBeDefined();
  });

  it("renders conversation titles", () => {
    render(<BravoConversationList conversations={[conv1, conv2]} />);
    expect(screen.getByText("First chat")).toBeDefined();
    expect(screen.getByText("Second chat")).toBeDefined();
  });

  it("shows 'Untitled' for conversations without a title", () => {
    const untitled = { ...conv1, title: "" };
    render(<BravoConversationList conversations={[untitled]} />);
    expect(screen.getByText("Untitled")).toBeDefined();
  });

  it("calls onNew when new conversation button is clicked", () => {
    const onNew = vi.fn();
    render(<BravoConversationList conversations={[]} onNew={onNew} />);
    fireEvent.click(screen.getByText("New conversation"));
    expect(onNew).toHaveBeenCalledTimes(1);
  });

  it("calls onSelect when a conversation is clicked", () => {
    const onSelect = vi.fn();
    render(<BravoConversationList conversations={[conv1]} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("First chat"));
    expect(onSelect).toHaveBeenCalledWith(conv1);
  });

  it("calls onDelete with the conversation", () => {
    const onDelete = vi.fn();
    render(<BravoConversationList conversations={[conv1]} onDelete={onDelete} />);
    const deleteBtn = screen.getByLabelText("Delete conversation");
    fireEvent.click(deleteBtn);
    expect(onDelete).toHaveBeenCalledWith(conv1);
  });
});

// ---------------------------------------------------------------------------
// BravoApprovalCard
// ---------------------------------------------------------------------------

describe("BravoApprovalCard", () => {
  it("renders tool name and approval required text", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={vi.fn()}
      />,
    );
    expect(screen.getByText("Approval required")).toBeDefined();
    expect(screen.getByText("translate")).toBeDefined();
  });

  it("renders input JSON when provided", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        input={{ text: "hello" }}
        onApprove={vi.fn()}
        onDeny={vi.fn()}
      />,
    );
    expect(screen.getByText(/"text": "hello"/)).toBeDefined();
  });

  it("calls onApprove with toolCallId", () => {
    const onApprove = vi.fn();
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={onApprove}
        onDeny={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByText("Approve"));
    expect(onApprove).toHaveBeenCalledWith("tc1");
  });

  it("calls onDeny with toolCallId", () => {
    const onDeny = vi.fn();
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={onDeny}
      />,
    );
    fireEvent.click(screen.getByText("Deny"));
    expect(onDeny).toHaveBeenCalledWith("tc1");
  });

  it("disables buttons when loading", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={vi.fn()}
        loading
      />,
    );
    const approveBtn = screen.getByText("Approve").closest("button");
    const denyBtn = screen.getByText("Deny").closest("button");
    expect(approveBtn?.disabled).toBe(true);
    expect(denyBtn?.disabled).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// BravoThread
// ---------------------------------------------------------------------------

describe("BravoThread", () => {
  it("shows empty state when no messages and not streaming", () => {
    render(<BravoThread messages={[]} />);
    expect(screen.getByText("Start a conversation with @bravo")).toBeDefined();
  });

  it("renders user and assistant messages", () => {
    render(<BravoThread messages={[userMsg, assistantMsg]} />);
    expect(screen.getByText("Hello bravo")).toBeDefined();
    expect(screen.getByText("Hi! How can I help?")).toBeDefined();
  });

  it("shows role labels", () => {
    render(<BravoThread messages={[userMsg, assistantMsg]} />);
    expect(screen.getByText("You")).toBeDefined();
    expect(screen.getByText("@bravo")).toBeDefined();
  });

  it("shows token usage for assistant messages", () => {
    render(<BravoThread messages={[assistantMsg]} />);
    expect(screen.getByText("10 in / 20 out tokens")).toBeDefined();
  });

  it("shows streaming content with cursor", () => {
    const { container } = render(
      <BravoThread messages={[]} streaming streamingContent="Thinking..." />,
    );
    expect(screen.getByText("Thinking...")).toBeDefined();
    // Cursor element should be present
    const cursor = container.querySelector(".animate-pulse");
    expect(cursor).not.toBeNull();
  });

  it("does not show streaming bubble when streamingContent is empty", () => {
    const { container } = render(<BravoThread messages={[]} streaming streamingContent="" />);
    // No streaming bubble rendered (no cursor), and no empty-state text either
    // because streaming=true suppresses the "Start a conversation" message.
    const cursor = container.querySelector(".animate-pulse");
    expect(cursor).toBeNull();
  });
});

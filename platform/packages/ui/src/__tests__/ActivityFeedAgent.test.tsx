import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { ActivityFeed } from "../components/ActivityFeed";
import type { ActivityInfo } from "../types/api";

function makeActivity(overrides: Partial<ActivityInfo> = {}): ActivityInfo {
  return {
    id: "a1",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "project.created",
    summary: "created a project",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("ActivityFeed — Agent events", () => {
  it("renders agent.conversation.created with custom summary", () => {
    const activity = makeActivity({
      type: "agent.conversation.created",
      summary: "",
      data: { title: "Translate French" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/started a conversation: "Translate French"/)).toBeDefined();
  });

  it("renders agent.message.sent with block count", () => {
    const activity = makeActivity({
      type: "agent.message.sent",
      summary: "",
      data: { blocks_count: "45" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/asked @bravo to process 45 blocks/)).toBeDefined();
  });

  it("renders agent.tool.executed with tool name", () => {
    const activity = makeActivity({
      type: "agent.tool.executed",
      summary: "",
      data: { tool: "run_flow", duration: "1.2s" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/@bravo ran run_flow/)).toBeDefined();
    expect(screen.getByText(/1\.2s/)).toBeDefined();
  });

  it("renders agent.tool.approved", () => {
    const activity = makeActivity({
      type: "agent.tool.approved",
      summary: "",
      data: { tool: "connector_push" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/approved @bravo to run connector_push/)).toBeDefined();
  });

  it("renders agent.tool.denied", () => {
    const activity = makeActivity({
      type: "agent.tool.denied",
      summary: "",
      data: { tool: "connector_push" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/denied @bravo from running connector_push/)).toBeDefined();
  });

  it("renders agent.code.executed with language", () => {
    const activity = makeActivity({
      type: "agent.code.executed",
      summary: "",
      data: { language: "python", exit_code: "0" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/@bravo ran a python script/)).toBeDefined();
  });

  it("renders agent.code.executed failure", () => {
    const activity = makeActivity({
      type: "agent.code.executed",
      summary: "",
      data: { language: "bash", exit_code: "1" },
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText(/@bravo ran a bash script \(failed\)/)).toBeDefined();
  });

  it("shows @bravo badge for agent activities", () => {
    const activity = makeActivity({
      type: "agent.message.sent",
      summary: "sent a message to @bravo",
    });
    const { container } = render(<ActivityFeed activities={[activity]} />);

    // Should have the @bravo badge span
    const bravoBadges = container.querySelectorAll(".text-purple-600");
    expect(bravoBadges.length).toBeGreaterThan(0);
  });

  it("uses @bravo as actor name for agent.tool.executed", () => {
    const activity = makeActivity({
      type: "agent.tool.executed",
      actor_name: "Alice",
      summary: "",
      data: { tool: "tm_search" },
    });
    render(<ActivityFeed activities={[activity]} />);

    // Actor should be "@bravo" not "Alice" for tool.executed events.
    // Use getAllByText since "@bravo" appears as both actor name and badge.
    const bravoElements = screen.getAllByText("@bravo");
    expect(bravoElements.length).toBeGreaterThanOrEqual(1);
    // The first one should be the actor name (font-medium span).
    expect(bravoElements[0].className).toContain("font-medium");
  });

  it("uses real actor name for agent.tool.approved", () => {
    const activity = makeActivity({
      type: "agent.tool.approved",
      actor_name: "Bob",
      summary: "",
      data: { tool: "connector_push" },
    });
    render(<ActivityFeed activities={[activity]} />);

    // Actor should be "Bob" (the human who approved)
    expect(screen.getByText("Bob")).toBeDefined();
  });

  it("falls back to server summary when provided", () => {
    const activity = makeActivity({
      type: "agent.conversation.created",
      summary: "Custom server summary",
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText("Custom server summary")).toBeDefined();
  });

  it("renders non-agent activities normally", () => {
    const activity = makeActivity({
      type: "project.created",
      summary: "created a new project",
    });
    render(<ActivityFeed activities={[activity]} />);

    expect(screen.getByText("Alice")).toBeDefined();
    expect(screen.getByText("created a new project")).toBeDefined();
  });
});

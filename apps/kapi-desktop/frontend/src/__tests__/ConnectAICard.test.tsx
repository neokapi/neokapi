import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConnectAICard } from "../components/ConnectAICard";
import type { AIDetectionResult } from "../types/api";
import { api } from "../hooks/useApi";

const detectedBoth: AIDetectionResult = {
  detected: [
    {
      provider: "claude-code",
      label: "Claude Code",
      model: "sonnet",
      detail: "signed in on this Mac · uses your Claude subscription",
      subscription: true,
    },
    {
      provider: "ollama",
      label: "Ollama",
      model: "llama3.2:3b",
      detail: "running locally · content stays on this machine",
      subscription: false,
    },
  ],
  configured: false,
};

const nothingDetected: AIDetectionResult = { detected: [], configured: false };

describe("ConnectAICard", () => {
  it("renders detected providers as one-click options", () => {
    render(<ConnectAICard detection={detectedBoth} />);
    expect(screen.getByText("Connect your AI")).toBeInTheDocument();
    expect(screen.getByTestId("connect-ai-claude-code")).toBeInTheDocument();
    expect(screen.getByText(/uses your Claude subscription/)).toBeInTheDocument();
    expect(screen.getByTestId("connect-ai-ollama")).toBeInTheDocument();
    // Escape hatches are always present.
    expect(screen.getByTestId("connect-ai-key-toggle")).toBeInTheDocument();
    expect(screen.getByTestId("connect-ai-skip-demo")).toBeInTheDocument();
  });

  it("renders nothing when a provider is already configured", () => {
    const { container } = render(<ConnectAICard detection={{ detected: [], configured: true }} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("selecting a detected provider persists it and confirms", async () => {
    const select = vi.spyOn(api, "selectAIProvider").mockResolvedValue(null);
    const onConfigured = vi.fn();
    render(<ConnectAICard detection={detectedBoth} onConfigured={onConfigured} />);

    await userEvent.click(screen.getByTestId("connect-ai-claude-code"));

    expect(select).toHaveBeenCalledWith("claude-code", "sonnet");
    expect(onConfigured).toHaveBeenCalledWith("Claude Code");
    expect(await screen.findByTestId("connect-ai-done")).toHaveTextContent(
      "kapi will translate with Claude Code",
    );
    select.mockRestore();
  });

  it("expands the API key form and saves through the vault", async () => {
    const save = vi.spyOn(api, "saveProvider").mockResolvedValue(null);
    const select = vi.spyOn(api, "selectAIProvider").mockResolvedValue(null);
    render(<ConnectAICard detection={nothingDetected} />);

    expect(screen.queryByTestId("connect-ai-key-form")).not.toBeInTheDocument();
    await userEvent.click(screen.getByTestId("connect-ai-key-toggle"));
    expect(screen.getByTestId("connect-ai-key-form")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("radio", { name: "OpenAI" }));
    await userEvent.type(screen.getByLabelText(/API key/), "sk-test-123");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({ provider_type: "openai", api_key: "sk-test-123" }),
    );
    expect(select).toHaveBeenCalledWith("openai", "");
    expect(await screen.findByTestId("connect-ai-done")).toBeInTheDocument();
    save.mockRestore();
    select.mockRestore();
  });

  it("skip link falls back to the demo engine", async () => {
    const select = vi.spyOn(api, "selectAIProvider").mockResolvedValue(null);
    render(<ConnectAICard detection={nothingDetected} />);

    await userEvent.click(screen.getByTestId("connect-ai-skip-demo"));

    expect(select).toHaveBeenCalledWith("demo", "");
    expect(await screen.findByTestId("connect-ai-done")).toBeInTheDocument();
    select.mockRestore();
  });
});

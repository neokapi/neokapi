import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { CredentialsPage } from "../components/CredentialsPage";
import type { AIModelOption } from "../types/api";

function renderWithProviders(ui: React.ReactElement) {
  return render(<ErrorProvider>{ui}</ErrorProvider>);
}

const sampleModels: AIModelOption[] = [
  {
    model: "llama3.2:3b",
    provider: "ollama",
    label: "Ollama",
    local: true,
    installed: false,
    needs_key: false,
    note: "default · smallest",
    is_default: true,
  },
  {
    model: "gpt-4o",
    provider: "openai",
    label: "OpenAI",
    local: false,
    installed: false,
    needs_key: true,
    is_default: false,
  },
];

describe("CredentialsPage (AI Models)", () => {
  it("renders the AI Models title and keychain notice", () => {
    renderWithProviders(
      <CredentialsPage providers={[]} providerTypes={[]} models={sampleModels} />,
    );
    expect(screen.getByText("AI Models")).toBeInTheDocument();
    expect(screen.getByText(/stored in your OS keychain/)).toBeInTheDocument();
  });

  it("renders the model picker and marks the default", () => {
    renderWithProviders(
      <CredentialsPage providers={[]} providerTypes={[]} models={sampleModels} />,
    );
    expect(screen.getByText("llama3.2:3b")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();

    // Exactly one model row is the selected default.
    const checked = screen
      .getAllByRole("radio")
      .filter((r) => r.getAttribute("aria-checked") === "true");
    expect(checked).toHaveLength(1);

    // The cloud model with no saved key is flagged.
    expect(screen.getByText("needs key")).toBeInTheDocument();
  });

  it("shows the cloud-keys empty state when no providers are saved", async () => {
    renderWithProviders(<CredentialsPage />);
    await waitFor(() => {
      expect(screen.getByText(/No cloud provider keys/)).toBeInTheDocument();
    });
  });

  it("shows add form when clicking Add Provider", async () => {
    renderWithProviders(<CredentialsPage providers={[]} providerTypes={[]} models={[]} />);
    await userEvent.click(screen.getByText("Add Provider"));
    expect(screen.getByText("New Provider")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("My Anthropic Key")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("sk-...")).toBeInTheDocument();
  });

  it("can cancel adding a provider", async () => {
    renderWithProviders(<CredentialsPage providers={[]} providerTypes={[]} models={[]} />);
    await userEvent.click(screen.getByText("Add Provider"));
    await userEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("New Provider")).not.toBeInTheDocument();
  });
});

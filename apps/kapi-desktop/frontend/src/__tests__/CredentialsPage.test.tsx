import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { CredentialsPage } from "../components/CredentialsPage";

function renderWithProviders(ui: React.ReactElement) {
  return render(<ErrorProvider>{ui}</ErrorProvider>);
}

describe("CredentialsPage", () => {
  it("renders title and keychain notice", async () => {
    renderWithProviders(<CredentialsPage />);
    expect(screen.getByText("AI Credentials")).toBeInTheDocument();
    expect(screen.getByText(/stored in your OS keychain/)).toBeInTheDocument();
  });

  it("shows empty state after loading", async () => {
    renderWithProviders(<CredentialsPage />);
    // Wait for async load to finish (api returns null outside Wails).
    await waitFor(() => {
      expect(screen.getByText(/No AI providers configured/)).toBeInTheDocument();
    });
  });

  it("shows add form when clicking Add Provider", async () => {
    renderWithProviders(<CredentialsPage />);
    await waitFor(() => {
      expect(screen.queryByText("Loading providers...")).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText("Add Provider"));
    expect(screen.getByText("New Provider")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("My Anthropic Key")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("sk-...")).toBeInTheDocument();
  });

  it("can cancel adding a provider", async () => {
    renderWithProviders(<CredentialsPage />);
    await waitFor(() => {
      expect(screen.queryByText("Loading providers...")).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText("Add Provider"));
    await userEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("New Provider")).not.toBeInTheDocument();
  });

  const twoProviders = [
    { id: "c1", name: "My OpenAI Key", provider_type: "openai" },
    { id: "c2", name: "My Gemini Key", provider_type: "gemini" },
  ];

  it("marks the default provider with a star and badge", () => {
    renderWithProviders(
      <CredentialsPage providers={twoProviders} providerTypes={[]} defaultCredentialId="c2" />,
    );
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Unset My Gemini Key as default/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: /Set My OpenAI Key as default/ })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("starring a provider makes it the default (optimistic)", async () => {
    renderWithProviders(
      <CredentialsPage providers={twoProviders} providerTypes={[]} defaultCredentialId="" />,
    );
    expect(screen.queryByText("Default")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Set My OpenAI Key as default/ }));

    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Unset My OpenAI Key as default/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});

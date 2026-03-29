import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { CredentialsPage } from "../components/CredentialsPage";

describe("CredentialsPage", () => {
  it("renders title and keychain notice", async () => {
    render(<CredentialsPage />);
    expect(screen.getByText("AI Credentials")).toBeInTheDocument();
    expect(screen.getByText(/stored in your OS keychain/)).toBeInTheDocument();
  });

  it("shows empty state after loading", async () => {
    render(<CredentialsPage />);
    // Wait for async load to finish (api returns null outside Wails).
    await waitFor(() => {
      expect(screen.getByText(/No AI providers configured/)).toBeInTheDocument();
    });
  });

  it("shows add form when clicking Add Provider", async () => {
    render(<CredentialsPage />);
    await waitFor(() => {
      expect(screen.queryByText("Loading providers...")).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText("Add Provider"));
    expect(screen.getByText("New Provider")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("My Anthropic Key")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("sk-...")).toBeInTheDocument();
  });

  it("can cancel adding a provider", async () => {
    render(<CredentialsPage />);
    await waitFor(() => {
      expect(screen.queryByText("Loading providers...")).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText("Add Provider"));
    await userEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("New Provider")).not.toBeInTheDocument();
  });
});

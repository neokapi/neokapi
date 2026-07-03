import { render, screen } from "@testing-library/react";
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

const sampleProviderTypes = [
  { name: "ollama", label: "Ollama", local: true },
  { name: "openai", label: "OpenAI" },
];

function renderPage() {
  return renderWithProviders(
    <CredentialsPage providers={[]} providerTypes={sampleProviderTypes} models={sampleModels} />,
  );
}

describe("CredentialsPage (AI Models)", () => {
  it("renders the AI Models title and keychain notice", () => {
    renderPage();
    expect(screen.getByText("AI Models")).toBeInTheDocument();
    expect(screen.getByText(/stored in your OS keychain/)).toBeInTheDocument();
  });

  it("groups models under a provider header and marks the default", () => {
    renderPage();
    // One group header per provider.
    expect(screen.getByRole("heading", { name: "Ollama" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "OpenAI" })).toBeInTheDocument();
    // Models under their providers.
    expect(screen.getByText("llama3.2:3b")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    // Exactly one model is the selected default.
    const checked = screen
      .getAllByRole("radio")
      .filter((r) => r.getAttribute("aria-checked") === "true");
    expect(checked).toHaveLength(1);
  });

  it("offers Add Credentials only for cloud providers (local needs none)", () => {
    renderPage();
    // OpenAI (cloud) has an Add Credentials affordance; Ollama (local) does not.
    expect(screen.getByRole("button", { name: /Add credentials for OpenAI/ })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Add credentials for Ollama/ }),
    ).not.toBeInTheDocument();
  });

  it("opens a provider-native key form (no provider chooser) when adding credentials", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /Add credentials for OpenAI/ }));
    // Title names the provider; there is no provider <select> to change it.
    expect(screen.getByText("Add OpenAI credentials")).toBeInTheDocument();
    expect(screen.queryByLabelText("Provider")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText("sk-...")).toBeInTheDocument();
    expect(screen.getByDisplayValue("OpenAI key")).toBeInTheDocument();
  });

  it("can cancel adding credentials", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /Add credentials for OpenAI/ }));
    await userEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("Add OpenAI credentials")).not.toBeInTheDocument();
  });

  it("lets you pick the default key when a provider has several", () => {
    const providers = [
      { id: "k1", name: "OpenAI key", provider_type: "openai", default: true },
      { id: "k2", name: "OpenAI key 2", provider_type: "openai", default: false },
    ];
    renderWithProviders(
      <CredentialsPage
        providers={providers}
        providerTypes={sampleProviderTypes}
        models={sampleModels}
      />,
    );
    // Each key is a default-selector; the marked one is pressed.
    expect(screen.getByRole("button", { name: /OpenAI key is the default key/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      screen.getByRole("button", { name: /Use OpenAI key 2 as the default key/ }),
    ).toHaveAttribute("aria-pressed", "false");
  });
});

describe("CredentialsPage detected providers", () => {
  const detection = {
    detected: [
      {
        provider: "claude-code",
        label: "Claude Code",
        model: "sonnet",
        detail: "signed in on this Mac · uses your Claude subscription",
        subscription: true,
      },
    ],
    configured: false,
  };

  const claudeCodeModels: AIModelOption[] = [
    {
      model: "sonnet",
      provider: "claude-code",
      label: "Claude Code",
      local: false,
      installed: true,
      needs_key: false,
      subscription: true,
      note: "uses your Claude subscription",
      is_default: false,
    },
    ...sampleModels,
  ];

  const typesWithClaude = [
    { name: "claude-code", label: "Claude Code", keyless: true, subscription: true },
    ...sampleProviderTypes,
  ];

  function renderDetected() {
    return renderWithProviders(
      <CredentialsPage
        providers={[]}
        providerTypes={typesWithClaude}
        models={claudeCodeModels}
        detection={detection}
      />,
    );
  }

  it("renders the Detected section with a Select button and no key field", () => {
    renderDetected();
    expect(screen.getByTestId("detected-providers")).toBeInTheDocument();
    const card = screen.getByTestId("detected-claude-code");
    expect(card).toHaveTextContent("Claude Code");
    expect(card).toHaveTextContent(/uses your Claude subscription/);
    expect(
      screen.getByRole("button", { name: /Use Claude Code as the default AI provider/ }),
    ).toBeInTheDocument();
  });

  it("keyless claude-code group shows no Add Credentials button", () => {
    renderDetected();
    // The OpenAI group (key-based) still offers key management…
    expect(screen.getAllByRole("button", { name: /Add credentials for OpenAI/ })).toHaveLength(1);
    // …but the keyless Claude Code group must not.
    expect(
      screen.queryByRole("button", { name: /Add credentials for Claude Code/ }),
    ).not.toBeInTheDocument();
    // And the subscription badge shows on the group header.
    expect(screen.getAllByText("uses your Claude subscription").length).toBeGreaterThan(0);
  });
});

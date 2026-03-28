import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrandProfileWizard } from "../brand/BrandProfileWizard";
import type { VoiceProfile } from "../brand/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeProfile(overrides: Partial<VoiceProfile> = {}): VoiceProfile {
  return {
    id: "prof-1",
    name: "Existing Brand",
    description: "An existing voice profile",
    tone: {
      personality: ["friendly", "helpful"],
      formality: "casual",
      emotion: "warm",
      humor: "light",
    },
    style: {
      active_voice: true,
      sentence_length: "short",
      person_pov: "second",
      contractions: "always",
    },
    vocabulary: {
      preferred_terms: [{ term: "help" }],
      forbidden_terms: [{ term: "synergy" }],
    },
    examples: [{ before: "Old text", after: "New text" }],
    workspace_id: "ws-1",
    version: 2,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-02T00:00:00Z",
    ...overrides,
  };
}

const defaultProps = {
  onSave: vi.fn(),
  onCancel: vi.fn(),
};

// ---------------------------------------------------------------------------
// BrandProfileWizard
// ---------------------------------------------------------------------------

describe("BrandProfileWizard", () => {
  it("renders step 1 (Identity) by default with name and description inputs", () => {
    render(<BrandProfileWizard {...defaultProps} />);
    expect(screen.getByText("Profile Identity")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
  });

  it("shows New Brand Voice Profile title when creating", () => {
    render(<BrandProfileWizard {...defaultProps} />);
    expect(screen.getByText("New Brand Voice Profile")).toBeInTheDocument();
  });

  it("shows Edit Profile title when editing an existing profile", () => {
    render(<BrandProfileWizard {...defaultProps} profile={makeProfile()} />);
    expect(screen.getByText("Edit Profile")).toBeInTheDocument();
  });

  it("Next button is disabled when name is empty on step 1", () => {
    render(<BrandProfileWizard {...defaultProps} />);
    const nextButton = screen.getByRole("button", { name: "Next" });
    expect(nextButton).toBeDisabled();
  });

  it("clicking Next advances to step 2 (Tone)", async () => {
    const user = userEvent.setup();
    render(<BrandProfileWizard {...defaultProps} />);
    // Fill in name first to enable Next
    const nameInput = screen.getByLabelText("Name");
    await user.type(nameInput, "My Brand");
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Tone & Personality")).toBeInTheDocument();
  });

  it("clicking Back returns to step 1", async () => {
    const user = userEvent.setup();
    render(<BrandProfileWizard {...defaultProps} />);
    const nameInput = screen.getByLabelText("Name");
    await user.type(nameInput, "My Brand");
    await user.click(screen.getByRole("button", { name: "Next" }));
    // Now on step 2
    expect(screen.getByText("Tone & Personality")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByText("Profile Identity")).toBeInTheDocument();
  });

  it("step nav buttons can be clicked to jump directly", async () => {
    const user = userEvent.setup();
    render(<BrandProfileWizard {...defaultProps} />);
    // Click the "Style" step button in the nav
    await user.click(screen.getByRole("button", { name: /Style/ }));
    expect(screen.getByText("Writing Style")).toBeInTheDocument();
  });

  it("Create button on last step calls onSave with correct data shape", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<BrandProfileWizard onSave={onSave} onCancel={vi.fn()} />);

    // Step 1: fill name
    const nameInput = screen.getByLabelText("Name");
    await user.type(nameInput, "Test Brand");
    const descInput = screen.getByLabelText("Description");
    await user.type(descInput, "A test description");

    // Jump to last step via nav
    await user.click(screen.getByRole("button", { name: /Vocabulary/ }));
    // Click Create Profile
    await user.click(screen.getByRole("button", { name: "Create Profile" }));

    expect(onSave).toHaveBeenCalledTimes(1);
    const saved = onSave.mock.calls[0][0];
    expect(saved.name).toBe("Test Brand");
    expect(saved.description).toBe("A test description");
    expect(saved.tone).toBeDefined();
    expect(saved.style).toBeDefined();
    expect(saved.vocabulary).toBeDefined();
    expect(saved.examples).toBeDefined();
  });

  it("pre-populates form when profile prop is provided", () => {
    const profile = makeProfile({
      name: "Pre-filled",
      description: "Already set",
    });
    render(<BrandProfileWizard {...defaultProps} profile={profile} />);
    expect(screen.getByDisplayValue("Pre-filled")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Already set")).toBeInTheDocument();
  });

  it("Cancel button calls onCancel", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(<BrandProfileWizard onSave={vi.fn()} onCancel={onCancel} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });
});

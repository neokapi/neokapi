import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PersonalityTagPicker } from "../brand/PersonalityTagPicker";

// ---------------------------------------------------------------------------
// PersonalityTagPicker
// ---------------------------------------------------------------------------

describe("PersonalityTagPicker", () => {
  it("renders existing tags as badges", () => {
    render(<PersonalityTagPicker tags={["friendly", "bold"]} onChange={vi.fn()} />);
    // The tags appear as badge text (selected area) AND as suggestion buttons.
    // The badge section renders first; verify they exist.
    expect(screen.getByLabelText("Remove friendly")).toBeInTheDocument();
    expect(screen.getByLabelText("Remove bold")).toBeInTheDocument();
  });

  it("clicking X on a tag removes it", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<PersonalityTagPicker tags={["friendly", "bold"]} onChange={handleChange} />);
    await user.click(screen.getByLabelText("Remove friendly"));
    expect(handleChange).toHaveBeenCalledWith(["bold"]);
  });

  it("clicking a suggested tag adds it", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<PersonalityTagPicker tags={[]} onChange={handleChange} />);
    // "professional" is the first suggestion in the Professional category
    await user.click(screen.getByRole("button", { name: "professional" }));
    expect(handleChange).toHaveBeenCalledWith(["professional"]);
  });

  it("already-selected tags appear disabled in suggestions", () => {
    render(<PersonalityTagPicker tags={["professional"]} onChange={vi.fn()} />);
    // The suggestion button for "professional" should be disabled
    const buttons = screen.getAllByRole("button", { name: "professional" });
    // One is the remove button inside the badge, the other is the suggestion
    const suggestionButton = buttons.find(
      (btn) => btn.tagName === "BUTTON" && btn.getAttribute("disabled") !== null,
    );
    expect(suggestionButton).toBeDefined();
    expect(suggestionButton).toBeDisabled();
  });

  it("typing a custom tag and pressing Enter adds it", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<PersonalityTagPicker tags={["friendly"]} onChange={handleChange} />);
    const input = screen.getByPlaceholderText("Type a custom tag and press Enter");
    await user.type(input, "innovative{Enter}");
    expect(handleChange).toHaveBeenCalledWith(["friendly", "innovative"]);
  });

  it("normalizes custom tags to lowercase", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<PersonalityTagPicker tags={[]} onChange={handleChange} />);
    const input = screen.getByPlaceholderText("Type a custom tag and press Enter");
    await user.type(input, "BOLD{Enter}");
    expect(handleChange).toHaveBeenCalledWith(["bold"]);
  });

  it("does not add duplicate tags", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<PersonalityTagPicker tags={["bold"]} onChange={handleChange} />);
    const input = screen.getByPlaceholderText("Type a custom tag and press Enter");
    await user.type(input, "bold{Enter}");
    // onChange should not be called because "bold" already exists
    expect(handleChange).not.toHaveBeenCalled();
  });

  it("renders category labels for suggestion groups", () => {
    render(<PersonalityTagPicker tags={[]} onChange={vi.fn()} />);
    expect(screen.getByText("Professional")).toBeInTheDocument();
    expect(screen.getByText("Friendly")).toBeInTheDocument();
    expect(screen.getByText("Creative")).toBeInTheDocument();
    expect(screen.getByText("Supportive")).toBeInTheDocument();
    expect(screen.getByText("Technical")).toBeInTheDocument();
  });
});

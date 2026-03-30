import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { EntityAnnotationDialog } from "@neokapi/ui-primitives";
import type { AnnotateResult, EntityPatternRequest } from "@neokapi/ui-primitives";

describe("EntityAnnotationDialog", () => {
  const defaultProps = {
    open: true,
    onClose: vi.fn(),
    selectedCount: 5,
    onApply: vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
      entries_updated: 3,
      entities_added: 7,
    }),
  };

  it("does not render when open is false", () => {
    const { container } = render(
      <EntityAnnotationDialog {...defaultProps} open={false} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders dialog with title and count", () => {
    render(<EntityAnnotationDialog {...defaultProps} />);
    expect(screen.getByText("Annotate Entities")).toBeInTheDocument();
    // The description paragraph mentions selected count and "entries"
    const description = screen.getByText(/5 selected/);
    expect(description).toBeInTheDocument();
    expect(description.textContent).toContain("entries");
  });

  it("renders singular entry text for count of 1", () => {
    render(<EntityAnnotationDialog {...defaultProps} selectedCount={1} />);
    expect(screen.getByText(/1 selected entry/)).toBeInTheDocument();
  });

  it("renders initial pattern from initialPattern prop", () => {
    render(<EntityAnnotationDialog {...defaultProps} initialPattern="Acme Corp" />);
    const input = screen.getByDisplayValue("Acme Corp");
    expect(input).toBeInTheDocument();
  });

  it("renders a pattern row with text input, entity type select, and case checkbox", () => {
    render(<EntityAnnotationDialog {...defaultProps} />);

    // Text input
    expect(screen.getByPlaceholderText("Text to match...")).toBeInTheDocument();

    // Entity type select
    const select = screen.getByDisplayValue("Person");
    expect(select).toBeInTheDocument();

    // Case sensitive checkbox
    expect(screen.getByText("Case")).toBeInTheDocument();
  });

  describe("add pattern", () => {
    it("adds a new pattern row when clicking '+ Add pattern'", async () => {
      render(<EntityAnnotationDialog {...defaultProps} />);

      // Initially 1 pattern row
      let inputs = screen.getAllByPlaceholderText("Text to match...");
      expect(inputs).toHaveLength(1);

      await userEvent.click(screen.getByText("+ Add pattern"));

      // Now 2 pattern rows
      inputs = screen.getAllByPlaceholderText("Text to match...");
      expect(inputs).toHaveLength(2);
    });
  });

  describe("remove pattern", () => {
    it("removes a pattern row when clicking x (only when multiple patterns)", async () => {
      render(<EntityAnnotationDialog {...defaultProps} />);

      // Add a second pattern
      await userEvent.click(screen.getByText("+ Add pattern"));
      let inputs = screen.getAllByPlaceholderText("Text to match...");
      expect(inputs).toHaveLength(2);

      // The x buttons should now be visible (only when > 1 pattern)
      const removeButtons = screen.getAllByText("x");
      expect(removeButtons.length).toBeGreaterThanOrEqual(1);

      await userEvent.click(removeButtons[0]);

      inputs = screen.getAllByPlaceholderText("Text to match...");
      expect(inputs).toHaveLength(1);
    });

    it("does not show remove button when only one pattern", () => {
      render(<EntityAnnotationDialog {...defaultProps} />);

      // With only 1 pattern, no x button should be shown
      const removeButtons = screen.queryAllByText("x");
      // The remove button only appears for patterns.length > 1
      expect(removeButtons).toHaveLength(0);
    });
  });

  describe("apply", () => {
    it("calls onApply with patterns when clicking apply", async () => {
      const onApply = vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
        entries_updated: 3,
        entities_added: 7,
      });

      render(
        <EntityAnnotationDialog
          {...defaultProps}
          onApply={onApply}
          initialPattern="Acme Corp"
        />,
      );

      await userEvent.click(screen.getByText("Apply to 5 entries"));

      await waitFor(() => {
        expect(onApply).toHaveBeenCalledWith([
          {
            text: "Acme Corp",
            entity_type: "entity:person",
            case_sensitive: true,
          },
        ]);
      });
    });

    it("filters out empty patterns before applying", async () => {
      const onApply = vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
        entries_updated: 1,
        entities_added: 1,
      });

      render(
        <EntityAnnotationDialog
          {...defaultProps}
          onApply={onApply}
          initialPattern="Valid Pattern"
        />,
      );

      // Add a second pattern but leave it empty
      await userEvent.click(screen.getByText("+ Add pattern"));

      await userEvent.click(screen.getByText("Apply to 5 entries"));

      await waitFor(() => {
        // Should only send the non-empty pattern
        expect(onApply).toHaveBeenCalledWith([
          expect.objectContaining({ text: "Valid Pattern" }),
        ]);
      });
    });

    it("does not call onApply when all patterns are empty", async () => {
      const onApply = vi.fn();

      render(
        <EntityAnnotationDialog {...defaultProps} onApply={onApply} initialPattern="" />,
      );

      // Button should be disabled since the pattern text is empty
      const applyBtn = screen.getByText("Apply to 5 entries");
      expect(applyBtn).toBeDisabled();
    });
  });

  describe("result display", () => {
    it("shows result after successful apply", async () => {
      const onApply = vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
        entries_updated: 3,
        entities_added: 7,
      });

      render(
        <EntityAnnotationDialog
          {...defaultProps}
          onApply={onApply}
          initialPattern="Acme"
        />,
      );

      await userEvent.click(screen.getByText("Apply to 5 entries"));

      await waitFor(() => {
        expect(screen.getByText(/Updated 3 entries/)).toBeInTheDocument();
        expect(screen.getByText(/added 7 entities/)).toBeInTheDocument();
      });

      // Done button should be visible
      expect(screen.getByText("Done")).toBeInTheDocument();
    });

    it("shows singular text for 1 entry and 1 entity", async () => {
      const onApply = vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
        entries_updated: 1,
        entities_added: 1,
      });

      render(
        <EntityAnnotationDialog
          {...defaultProps}
          onApply={onApply}
          initialPattern="Test"
        />,
      );

      await userEvent.click(screen.getByText("Apply to 5 entries"));

      await waitFor(() => {
        expect(screen.getByText(/Updated 1 entry/)).toBeInTheDocument();
        expect(screen.getByText(/added 1 entity\./)).toBeInTheDocument();
      });
    });

    it("clicking Done calls onClose", async () => {
      const onClose = vi.fn();
      const onApply = vi.fn<(patterns: EntityPatternRequest[]) => Promise<AnnotateResult>>().mockResolvedValue({
        entries_updated: 1,
        entities_added: 1,
      });

      render(
        <EntityAnnotationDialog
          {...defaultProps}
          onClose={onClose}
          onApply={onApply}
          initialPattern="Test"
        />,
      );

      await userEvent.click(screen.getByText("Apply to 5 entries"));

      await waitFor(() => {
        expect(screen.getByText("Done")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Done"));
      expect(onClose).toHaveBeenCalled();
    });
  });

  describe("entity type selection", () => {
    it("allows changing entity type", async () => {
      render(<EntityAnnotationDialog {...defaultProps} initialPattern="New York" />);

      const select = screen.getByDisplayValue("Person");
      await userEvent.selectOptions(select, "entity:location");

      expect((select as HTMLSelectElement).value).toBe("entity:location");
    });
  });

  describe("case sensitivity", () => {
    it("toggles case sensitivity", async () => {
      render(<EntityAnnotationDialog {...defaultProps} initialPattern="test" />);

      const checkbox = screen.getByRole("checkbox");
      // Initially checked (case_sensitive = true)
      expect(checkbox).toBeChecked();

      await userEvent.click(checkbox);
      expect(checkbox).not.toBeChecked();
    });
  });

  describe("cancel", () => {
    it("calls onClose when Cancel is clicked", async () => {
      const onClose = vi.fn();
      render(<EntityAnnotationDialog {...defaultProps} onClose={onClose} />);

      await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(onClose).toHaveBeenCalled();
    });
  });
});

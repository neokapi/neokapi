import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import type { CheckRunResult } from "../types/api";

const FAILING: CheckRunResult = {
  pass: false,
  score: 64,
  files: [
    {
      path: "src/locales/en.json",
      findings: [
        {
          category: "do-not-translate",
          severity: "critical",
          message: 'Do-not-translate term "Acme Cloud" is missing from the de target',
          suggestion: 'Keep "Acme Cloud" verbatim in the target',
          original_text: "Acme Cloud",
          block_id: "blk-1",
          field: "target",
          fixable: false,
        },
        {
          category: "vocabulary",
          severity: "major",
          message: 'Forbidden term "utilize" found',
          suggestion: 'Use "use" instead',
          original_text: "utilize",
          replacement: "use",
          block_id: "blk-2",
          field: "source",
          fixable: true,
        },
      ],
    },
  ],
};

const PASSING: CheckRunResult = {
  pass: true,
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

function renderPanel(props: Partial<React.ComponentProps<typeof ChecksPanel>> = {}) {
  return render(
    <ErrorProvider>
      <ChecksPanel tabID="t1" targetLanguages={["de"]} {...props} />
    </ErrorProvider>,
  );
}

describe("ChecksPanel", () => {
  it("renders the panel title and run action", () => {
    renderPanel();
    expect(screen.getByText("Checks")).toBeInTheDocument();
    // Idle state shows the header action plus an empty-state prompt; both are
    // "Run checks" buttons.
    expect(screen.getAllByRole("button", { name: /Run checks/i }).length).toBeGreaterThanOrEqual(1);
  });

  it("renders score and verdict for a failing run", () => {
    renderPanel({ result: FAILING });
    expect(screen.getByText("64")).toBeInTheDocument();
    expect(screen.getByText("Failing")).toBeInTheDocument();
  });

  it("renders each finding's message, category, offending text and suggestion", () => {
    renderPanel({ result: FAILING });
    expect(screen.getByText('Forbidden term "utilize" found')).toBeInTheDocument();
    expect(
      screen.getByText('Do-not-translate term "Acme Cloud" is missing from the de target'),
    ).toBeInTheDocument();
    // Category badges.
    expect(screen.getByText("vocabulary")).toBeInTheDocument();
    expect(screen.getByText("do-not-translate")).toBeInTheDocument();
    // Severity badges.
    expect(screen.getByText("Critical")).toBeInTheDocument();
    expect(screen.getByText("Major")).toBeInTheDocument();
    // Offending text + suggestion.
    expect(screen.getByText("utilize")).toBeInTheDocument();
    expect(screen.getByText('Use "use" instead')).toBeInTheDocument();
  });

  it("shows an Apply fix button only for fixable findings", () => {
    renderPanel({ result: FAILING });
    // FAILING has exactly one fixable finding.
    const fixButtons = screen.getAllByRole("button", { name: /Apply fix/i });
    expect(fixButtons).toHaveLength(1);
  });

  it("calls the fix handler with the finding when Apply fix is clicked", async () => {
    const onApplyFix = vi.fn().mockResolvedValue(undefined);
    renderPanel({ result: FAILING, onApplyFix });
    await userEvent.click(screen.getByRole("button", { name: /Apply fix/i }));
    await waitFor(() => expect(onApplyFix).toHaveBeenCalledTimes(1));
    const [filePath, finding] = onApplyFix.mock.calls[0];
    expect(filePath).toBe("src/locales/en.json");
    expect(finding.original_text).toBe("utilize");
    expect(finding.replacement).toBe("use");
    expect(finding.block_id).toBe("blk-2");
  });

  it("renders the all-clear state for a passing run with no findings", () => {
    renderPanel({ result: PASSING });
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByText(/No findings\. Your content passes all checks\./i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Apply fix/i })).not.toBeInTheDocument();
  });

  it("shows the loading state when forceLoading is set", () => {
    renderPanel({ forceLoading: true });
    expect(screen.getByRole("button", { name: /Running\.\.\./i })).toBeInTheDocument();
    // No verdict card while loading.
    expect(screen.queryByText("Passing")).not.toBeInTheDocument();
  });

  it("shows the idle empty state before any run", () => {
    renderPanel();
    expect(
      screen.getByText(/Run checks to verify your content against terminology/i),
    ).toBeInTheDocument();
  });
});

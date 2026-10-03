import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import type { CheckRunResult } from "../types/api";
import { MemoryChanges } from "../stories/memoryChanges";

/** The source block the fixable finding sits on, as the change service reads it. */
function sourceBlocks(text = "Please utilize the dashboard") {
  return new MemoryChanges([{ doc: "src/locales/en.json", block: "greeting", text }]);
}
const REV = sourceBlocks().rev({
  doc: "src/locales/en.json",
  block: "greeting",
  text: "Please utilize the dashboard",
});

const FAILING: CheckRunResult = {
  pass: false,
  verdict: "failed",
  score: 64,
  files: [
    {
      path: "src/locales/en.json",
      findings: [
        {
          category: "do-not-translate",
          fails: true,
          message: 'Do-not-translate term "Acme Cloud" is missing from the de target',
          suggestion: 'Keep "Acme Cloud" verbatim in the target',
          original_text: "Acme Cloud",
          block_id: "blk-1",
          field: "target",
        },
        {
          category: "vocabulary",
          fails: true,
          message: 'Forbidden term "utilize" found',
          suggestion: 'Use "use" instead',
          original_text: "utilize",
          replacement: "use",
          block_id: "blk-2",
          field: "source",
          fix: JSON.stringify({
            op: "replace_text",
            at: { doc: "src/locales/en.json", block: "greeting" },
            if_match: REV,
            edits: [
              { range: { start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } }, text: "use" },
            ],
          }),
        },
      ],
    },
  ],
};

const PASSING: CheckRunResult = {
  pass: true,
  verdict: "passed",
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const NOTHING_TO_CHECK: CheckRunResult = {
  pass: false,
  verdict: "did_not_run",
  did_not_run_cause: "nothing_to_check",
  did_not_run: ["no content blocks were checked"],
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const CHECKER_INVALID: CheckRunResult = {
  pass: false,
  verdict: "did_not_run",
  did_not_run_cause: "checker_invalid",
  did_not_run: ["placeholder reported no finding on its canary on src/locales/en.json (de)"],
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const PASSING_WITH_WARNINGS: CheckRunResult = {
  ...PASSING,
  warnings: [
    {
      code: "voice.unknown_key",
      message:
        'unknown key "preffered_terms" (line 7) is ignored when the profile loads; check its spelling and the section it sits under',
      source: ".kapi/voice.yaml",
      key: "vocabulary.preffered_terms",
    },
  ],
};

function renderPanel(props: Partial<React.ComponentProps<typeof ChecksPanel>> = {}) {
  return render(
    <ErrorProvider>
      <ChecksPanel tabID="t1" {...props} />
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
    // Outcome badges: both findings fail.
    expect(screen.getAllByText("Fails")).toHaveLength(2);
    // Offending text + suggestion.
    expect(screen.getByText("utilize")).toBeInTheDocument();
    expect(screen.getByText('Use "use" instead')).toBeInTheDocument();
  });

  it("places a comment finding on its block and lines", () => {
    const COMMENT_FINDINGS: CheckRunResult = {
      pass: false,
      verdict: "failed",
      score: 94,
      files: [
        {
          path: "code/parse.go",
          findings: [
            {
              category: "hygiene",
              fails: false,
              message: 'Content contains a doubled word: "the"',
              block_id: "func/Parse",
              field: "source",
              rule: "hygiene.doubled-word",
              lines: { first: 3, last: 4 },
            },
            {
              category: "voice",
              fails: true,
              message: "Prohibited pattern: Do not leave a FIXME without an owner.",
              block_id: "func/Retry",
              field: "source",
              rule: "voice.style",
              lines: { first: 7, last: 7 },
            },
          ],
        },
      ],
    };
    renderPanel({ result: COMMENT_FINDINGS });
    const locations = screen.getAllByTestId("finding-location");
    expect(locations).toHaveLength(2);
    expect(locations[0]).toHaveTextContent("func/Parse");
    expect(locations[0]).toHaveTextContent("lines 3–4");
    expect(locations[1]).toHaveTextContent("func/Retry");
    expect(locations[1]).toHaveTextContent("line 7");
  });

  it("renders a finding's offending text in its own locale's direction", () => {
    const RTL_FINDING: CheckRunResult = {
      pass: false,
      score: 80,
      files: [
        {
          path: "src/locales/ar.json",
          findings: [
            {
              category: "do-not-translate",
              fails: true,
              message: 'Do-not-translate term "Acme Cloud" is missing from the ar target',
              original_text: "أكمي كلاود",
              block_id: "blk-1",
              field: "target",
              locale: "ar-EG",
            },
          ],
        },
      ],
    };
    renderPanel({ result: RTL_FINDING });
    const found = screen.getByText("أكمي كلاود");
    expect(found).toHaveAttribute("dir", "rtl");
    expect(found).toHaveAttribute("lang", "ar-EG");
  });

  it("shows an Apply fix button only for findings that carry a fix", () => {
    renderPanel({ result: FAILING });
    // FAILING has exactly one finding with a fix.
    const fixButtons = screen.getAllByRole("button", { name: /Apply fix/i });
    expect(fixButtons).toHaveLength(1);
  });

  it("sends the finding's fix to the change service as it is", async () => {
    const changes = sourceBlocks();
    renderPanel({ result: FAILING, changes });
    await userEvent.click(screen.getByRole("button", { name: /Apply fix/i }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const fix = JSON.parse(FAILING.files[0].findings[1].fix!);
    expect(changes.sets[0].ops).toEqual([fix]);
    expect(screen.queryByTestId("finding-fix-refused")).not.toBeInTheDocument();
  });

  it("asks before applying a fix to text that changed since the check", async () => {
    const changes = sourceBlocks();
    changes.touch("src/locales/en.json", "greeting", "Please do utilize the dashboard");
    renderPanel({ result: FAILING, changes });
    await userEvent.click(screen.getByRole("button", { name: /Apply fix/i }));
    const prompt = await screen.findByText("Changed since you opened it");
    expect(prompt).toBeInTheDocument();
    expect(screen.getByText("Please do utilize the dashboard")).toBeInTheDocument();

    // Applied again, the fix finds its words in the text as it stands.
    await userEvent.click(screen.getByRole("button", { name: /Apply the fix to this text/i }));
    await waitFor(() => expect(changes.sets).toHaveLength(2));
    const again = changes.sets[1].ops[0];
    expect(again).toMatchObject({
      op: "replace_text",
      edits: [{ find: "utilize", text: "use" }],
    });
    expect(again.op === "replace_text" && again.if_match).not.toBe(REV);
    await waitFor(() =>
      expect(screen.queryByText("Changed since you opened it")).not.toBeInTheDocument(),
    );
  });

  // The words were found again across a bold span someone added since the
  // check: sent again, the plain-text replacement would delete the bold, so the
  // prompt offers only to keep the current text.
  it("offers no fix over text whose words now have formatting among them", async () => {
    const changes = sourceBlocks();
    changes.touch(
      "src/locales/en.json",
      "greeting",
      'Please util<x id="1"/>ize<x id="/1"/> the dashboard',
    );
    renderPanel({ result: FAILING, changes });
    await userEvent.click(screen.getByRole("button", { name: /Apply fix/i }));
    await screen.findByText("Changed since you opened it");
    expect(
      screen.queryByRole("button", { name: /Apply the fix to this text/i }),
    ).not.toBeInTheDocument();
    expect(document.querySelector("[data-slot='stale-prompt-note']")).toHaveTextContent(
      /formatting now sits among them/,
    );
    expect(changes.sets).toHaveLength(1);
  });

  it("renders the all-clear state for a passing run with no findings", () => {
    renderPanel({ result: PASSING });
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByText(/No findings\. Your content passes all checks\./i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Apply fix/i })).not.toBeInTheDocument();
  });

  it("shows a run with nothing to check as did not run, never as passing", () => {
    renderPanel({ result: NOTHING_TO_CHECK });
    expect(screen.getByText("Did not run")).toBeInTheDocument();
    expect(screen.getAllByText("There was nothing in scope to check.").length).toBeGreaterThan(0);
    expect(screen.getByText("no content blocks were checked")).toBeInTheDocument();
    expect(screen.queryByText("Passing")).not.toBeInTheDocument();
    expect(screen.queryByText(/Your content passes all checks/i)).not.toBeInTheDocument();
    expect(screen.queryByText("100")).not.toBeInTheDocument();
  });

  it("tells a broken checker apart from nothing to check", () => {
    renderPanel({ result: CHECKER_INVALID });
    expect(screen.getByText("Did not run")).toBeInTheDocument();
    expect(
      screen.getAllByText("A checker failed its canary, so this run's result cannot be trusted.")
        .length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("There was nothing in scope to check.")).not.toBeInTheDocument();
    expect(screen.queryByText("Passing")).not.toBeInTheDocument();
  });

  it("lists configuration warnings apart from the findings and keeps the verdict", () => {
    renderPanel({ result: PASSING_WITH_WARNINGS });
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByText(/No findings\. Your content passes all checks\./i)).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("Configuration warnings")).toBeInTheDocument();
    expect(screen.getByText("voice.unknown_key")).toBeInTheDocument();
    expect(screen.getByText(".kapi/voice.yaml: vocabulary.preffered_terms")).toBeInTheDocument();
    expect(screen.getAllByTestId("check-warning")).toHaveLength(1);
    expect(screen.queryAllByTestId("finding-card")).toHaveLength(0);
  });

  it("shows no warnings section for a run without warnings", () => {
    renderPanel({ result: FAILING });
    expect(screen.queryByTestId("check-warnings")).not.toBeInTheDocument();
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

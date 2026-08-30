import { render, screen, waitFor } from "./testUtils";
import { describe, it, expect, vi } from "vitest";

// The runner reads the tool registry to decide whether the flow it is about to
// run produces a target variant. Mock the bridge so that answer is deterministic.
const listProjectTools = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    listProjectTools: (...args: unknown[]) => listProjectTools(...args),
    getRunState: () => Promise.resolve("idle"),
  },
}));

import { RunnerPage } from "../components/RunnerPage";
import { JobFeedProvider } from "../context/JobFeedContext";

const MONOLINGUAL = [
  { name: "word-count", cardinality: "monolingual" },
  { name: "encoding-detect", cardinality: "monolingual" },
];
const BILINGUAL = [
  { name: "word-count", cardinality: "monolingual" },
  { name: "translate", cardinality: "bilingual" },
];

function renderRunner(steps: Array<{ tool: string }>) {
  return render(
    <JobFeedProvider>
      <RunnerPage tabID="t1" flowName="f" flow={{ steps }} onClose={vi.fn()} />
    </JobFeedProvider>,
  );
}

describe("RunnerPage target language", () => {
  it("asks nothing when every step is monolingual", async () => {
    listProjectTools.mockResolvedValue(MONOLINGUAL);
    renderRunner([{ tool: "word-count" }, { tool: "encoding-detect" }]);

    await waitFor(() => expect(listProjectTools).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByText("Target Language")).not.toBeInTheDocument());
    expect(screen.queryByPlaceholderText("e.g. fr")).not.toBeInTheDocument();
  });

  it("asks when a step produces a target variant", async () => {
    listProjectTools.mockResolvedValue(BILINGUAL);
    renderRunner([{ tool: "word-count" }, { tool: "translate" }]);

    await waitFor(() => expect(screen.getByText("Target Language")).toBeInTheDocument());
  });

  it("asks while the registry answer is still unknown", () => {
    // An unresolved query must not be read as "monolingual" — that would launch
    // a bilingual flow with no target.
    listProjectTools.mockReturnValue(new Promise(() => {}));
    renderRunner([{ tool: "translate" }]);

    expect(screen.getByText("Target Language")).toBeInTheDocument();
  });

  it("asks for a tool the registry does not list", async () => {
    listProjectTools.mockResolvedValue(MONOLINGUAL);
    renderRunner([{ tool: "word-count" }, { tool: "some-plugin-tool" }]);

    await waitFor(() => expect(listProjectTools).toHaveBeenCalled());
    expect(screen.getByText("Target Language")).toBeInTheDocument();
  });
});

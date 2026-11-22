import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";

// The runner reads the tool registry to decide whether the flow it is about to
// run produces a target variant. Mock the bridge so that answer is deterministic.
const listProjectTools = vi.fn();
const runFlow = vi.fn().mockResolvedValue(null);
vi.mock("../hooks/useApi", () => ({
  api: {
    listProjectTools: (...args: unknown[]) => listProjectTools(...args),
    matchContent: async () => [{ path: "/a.md" }, { path: "/b.md" }],
    checkAISetup: async () => ({ kind: "" }),
    runFlow: (...args: unknown[]) => runFlow(...args),
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

  it("runs a source-only project automatically when every step is monolingual", async () => {
    listProjectTools.mockResolvedValue(MONOLINGUAL);
    render(
      <JobFeedProvider>
        <RunnerPage
          tabID="auto"
          flowName="count"
          project={{ name: "Source", version: "v1" }}
          flow={{ steps: [{ tool: "word-count" }] }}
          autoRun
          onClose={vi.fn()}
        />
      </JobFeedProvider>,
    );
    await waitFor(() =>
      expect(runFlow).toHaveBeenCalledWith("auto", "count", ["/a.md", "/b.md"], []),
    );
  });

  it("runs the project files chosen in the manual input control", async () => {
    listProjectTools.mockResolvedValue(MONOLINGUAL);
    render(
      <JobFeedProvider>
        <RunnerPage
          tabID="manual"
          flowName="count"
          project={{ name: "Source", version: "v1" }}
          flow={{ steps: [{ tool: "word-count" }] }}
          onClose={vi.fn()}
        />
      </JobFeedProvider>,
    );
    await userEvent.click(await screen.findByText("2 files selected"));
    await userEvent.click(await screen.findByRole("menuitemcheckbox", { name: "/a.md" }));
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await waitFor(() => expect(runFlow).toHaveBeenCalledWith("manual", "count", ["/b.md"], []));
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

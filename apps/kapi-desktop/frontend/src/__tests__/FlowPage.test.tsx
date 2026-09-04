import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Events } from "@wailsio/runtime";
import type { FlowTrace } from "@neokapi/flow-editor";
import type { FlowSpec, RunTraces } from "../types/api";

const listRunTraces = vi.fn();
const getLastTrace = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    listTools: vi.fn().mockResolvedValue([]),
    listProjectTools: vi.fn().mockResolvedValue([]),
    getToolSchema: vi.fn().mockResolvedValue(null),
    listProviders: vi.fn().mockResolvedValue([]),
    listRunTraces: (...args: unknown[]) => listRunTraces(...args),
    getLastTrace: (...args: unknown[]) => getLastTrace(...args),
  },
}));

import { FlowPage } from "../components/FlowPage";

const LINEAR: FlowSpec = { steps: [{ tool: "translate" }, { tool: "qa" }] };
const FILE = "/p/src/messages.json";
const OTHER = "/p/src/other.json";

const RUN: RunTraces = {
  flow_name: "translate",
  steps: [{ tool: "translate" }, { tool: "qa" }],
  files: [{ file_path: FILE, locale: "fr-FR", output_path: "/p/out/fr-FR/messages.json" }],
  max_parts: 500,
};

const TRACE: FlowTrace = {
  name: "translate",
  nodes: [
    { id: "reader", type: "reader", name: "json" },
    { id: "tool-0", type: "tool", name: "translate" },
    { id: "tool-1", type: "tool", name: "qa" },
    { id: "writer", type: "writer", name: "json" },
  ],
  events: [
    { ts: 1, type: "enter", nodeId: "tool-0", partId: "b1" },
    { ts: 2, type: "exit", nodeId: "tool-0", partId: "b1" },
  ],
  parts: {
    b1: {
      initial: { id: "b1", type: "Block", summary: "Hello", sourceText: "Hello" },
      afterNode: {},
    },
  },
  durationUs: 2,
};

function renderPage(flow: FlowSpec = LINEAR, flowName = "translate") {
  return render(<FlowPage flowName={flowName} flow={flow} onChange={vi.fn()} onRun={vi.fn()} />);
}

const pressed = (testId: string) =>
  screen.getByTestId(testId).getAttribute("aria-pressed") === "true";

/** Resolve the handler the page registered for a Wails event. */
async function handlerFor(name: string): Promise<(e: { data: unknown }) => void> {
  const on = vi.mocked(Events.On);
  return await waitFor(() => {
    const call = on.mock.calls.find((c) => c[0] === name);
    if (!call) throw new Error(`no subscription for ${name}`);
    return call[1] as (e: { data: unknown }) => void;
  });
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("FlowPage", () => {
  beforeEach(() => {
    listRunTraces.mockReset();
    listRunTraces.mockResolvedValue(null);
    getLastTrace.mockReset();
    getLastTrace.mockResolvedValue(TRACE);
    vi.mocked(Events.On).mockClear();
  });

  it("opens on the step editor with a Steps / Diagram switch, and no Run view before a run", async () => {
    renderPage({
      steps: [
        { tool: "translate" },
        { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
      ],
    });
    // The linear flow editor renders a Run button in its header.
    expect(screen.getByLabelText("Run flow")).toBeInTheDocument();
    expect(pressed("flow-view-steps")).toBe(true);
    expect(screen.getByTestId("flow-view-diagram")).toBeInTheDocument();
    await waitFor(() => expect(listRunTraces).toHaveBeenCalled());
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
    expect(getLastTrace).not.toHaveBeenCalled();
  });

  it("shows the flow as a read-only diagram, parallel branches included", async () => {
    renderPage({
      steps: [
        { tool: "translate" },
        { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
      ],
    });
    await userEvent.click(screen.getByTestId("flow-view-diagram"));
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("qa")).toBeInTheDocument();
    expect(screen.getByText("word-count")).toBeInTheDocument();
    expect(screen.queryByLabelText("Add tool")).not.toBeInTheDocument();
    expect(screen.queryByText("Add branch")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Remove tool")).not.toBeInTheDocument();
  });

  it("replays the retained run of this flow in the Run view, naming its file", async () => {
    listRunTraces.mockResolvedValue(RUN);
    renderPage();

    await screen.findByTestId("flow-view-run");
    expect(pressed("flow-view-run")).toBe(true);
    expect(getLastTrace).toHaveBeenCalledWith(FILE, "fr-FR");
    await screen.findByLabelText("Playback position");
    expect(screen.getByTestId("run-trace-file")).toHaveTextContent("messages.json · fr-FR");
    expect(screen.queryByTestId("run-trace-truncated")).not.toBeInTheDocument();
  });

  it("withholds a run whose steps differ from the flow shown", async () => {
    listRunTraces.mockResolvedValue({ ...RUN, steps: [{ tool: "translate" }] });
    renderPage();
    await waitFor(() => expect(listRunTraces).toHaveBeenCalled());
    await tick();
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
    expect(getLastTrace).not.toHaveBeenCalled();
  });

  it("withholds another flow's run", async () => {
    listRunTraces.mockResolvedValue({ ...RUN, flow_name: "check" });
    renderPage();
    await waitFor(() => expect(listRunTraces).toHaveBeenCalled());
    await tick();
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
    expect(getLastTrace).not.toHaveBeenCalled();
  });

  it("loads the run when the backend reports a trace, and ignores progress ticks", async () => {
    renderPage();
    await waitFor(() => expect(listRunTraces).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();

    const onFlowEvent = await handlerFor("flow:event");
    listRunTraces.mockResolvedValue(RUN);

    onFlowEvent({ data: { type: "pipeline_metrics", flow_id: "translate", steps: [] } });
    await tick();
    expect(listRunTraces).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();

    onFlowEvent({
      data: { type: "trace", flow_id: "translate", file_path: FILE, locale: "fr-FR" },
    });
    await screen.findByTestId("flow-view-run");
    expect(pressed("flow-view-run")).toBe(true);
    expect(getLastTrace).toHaveBeenCalledWith(FILE, "fr-FR");
  });

  it("lets the reader pick another file of the run", async () => {
    listRunTraces.mockResolvedValue({
      ...RUN,
      files: [
        { file_path: OTHER, locale: "fr-FR" },
        { file_path: FILE, locale: "fr-FR" },
      ],
    });
    renderPage();
    await screen.findByTestId("flow-view-run");
    // The file that completed last replays first.
    await waitFor(() => expect(getLastTrace).toHaveBeenCalledWith(FILE, "fr-FR"));
    const picker = screen.getByTestId("run-trace-file");
    expect(picker).toHaveTextContent("messages.json · fr-FR");

    await userEvent.click(picker);
    await userEvent.click(await screen.findByRole("option", { name: "other.json · fr-FR" }));
    await waitFor(() => expect(getLastTrace).toHaveBeenCalledWith(OTHER, "fr-FR"));
    expect(screen.getByTestId("run-trace-file")).toHaveTextContent("other.json · fr-FR");
  });

  it("says when the recording budget cut the trace short", async () => {
    listRunTraces.mockResolvedValue({
      ...RUN,
      files: [{ file_path: FILE, locale: "fr-FR", truncated: true }],
      max_parts: 500,
    });
    renderPage();
    await screen.findByTestId("flow-view-run");
    expect(await screen.findByTestId("run-trace-truncated")).toHaveTextContent("First 500 parts");
  });

  it("hides a dismissed run until the retained set changes", async () => {
    listRunTraces.mockResolvedValue(RUN);
    renderPage();
    await screen.findByTestId("flow-view-run");
    await userEvent.click(await screen.findByLabelText("Dismiss the run"));
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
    expect(pressed("flow-view-diagram")).toBe(true);

    // Another file of the run completes: the run is offered again.
    listRunTraces.mockResolvedValue({
      ...RUN,
      files: [...RUN.files, { file_path: OTHER, locale: "fr-FR" }],
    });
    const onFlowEvent = await handlerFor("flow:event");
    onFlowEvent({
      data: { type: "trace", flow_id: "translate", file_path: OTHER, locale: "fr-FR" },
    });
    await screen.findByTestId("flow-view-run");
  });
});

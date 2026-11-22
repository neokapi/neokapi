import { render, screen } from "./testUtils";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import type { Job } from "../context/JobFeedContext";

const feed = vi.hoisted(() => ({ jobs: [] as Job[] }));

vi.mock("../context/JobFeedContext", () => ({
  useJobFeed: () => ({
    jobs: feed.jobs,
    hasActive: feed.jobs.some((j) => j.status === "running"),
    selectJob: vi.fn(),
    clearJob: vi.fn(),
    clearAll: vi.fn(),
  }),
}));

import { JobFeedButton } from "../components/JobFeedButton";
import { AIActivityButton } from "../components/AIActivityButton";
import { api } from "../hooks/useApi";
import type { AIActivityEntry } from "../types/api";

afterEach(() => {
  feed.jobs = [];
  vi.restoreAllMocks();
});

describe("JobFeedButton", () => {
  it("draws nothing while the session has no job", () => {
    render(<JobFeedButton />);
    expect(screen.queryByRole("button", { name: "Job feed" })).toBeNull();
  });

  it("appears once a job has run", () => {
    feed.jobs = [
      {
        id: "j1",
        flowName: "translate",
        status: "complete",
        startedAt: Date.now(),
        progress: { current: 1, total: 1 },
      } as unknown as Job,
    ];
    render(<JobFeedButton />);
    expect(screen.getByRole("button", { name: "Job feed" })).toBeInTheDocument();
  });
});

describe("AIActivityButton", () => {
  it("draws nothing while the session has made no model call", async () => {
    const probe = vi
      .spyOn(api, "getAIActivity")
      .mockResolvedValue({ entries: [], dropped: 0, cap: 500 });
    render(<AIActivityButton />);
    await vi.waitFor(() => expect(probe).toHaveBeenCalledWith(1));
    expect(screen.queryByRole("button", { name: "AI activity" })).toBeNull();
  });

  it("appears once the session has made a model call", async () => {
    vi.spyOn(api, "getAIActivity").mockResolvedValue({
      entries: [{ id: 1 } as unknown as AIActivityEntry],
      dropped: 0,
      cap: 500,
    });
    render(<AIActivityButton />);
    expect(await screen.findByRole("button", { name: "AI activity" })).toBeInTheDocument();
  });

  it("keeps asking until the first call arrives", async () => {
    const probe = vi
      .spyOn(api, "getAIActivity")
      .mockResolvedValueOnce({ entries: [], dropped: 0, cap: 500 })
      .mockResolvedValue({
        entries: [{ id: 1 } as unknown as AIActivityEntry],
        dropped: 0,
        cap: 500,
      });
    render(<AIActivityButton probeMs={10} />);
    expect(await screen.findByRole("button", { name: "AI activity" })).toBeInTheDocument();
    expect(probe.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});

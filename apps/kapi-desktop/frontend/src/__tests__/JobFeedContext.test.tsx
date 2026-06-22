import { render, screen, act, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { JobFeedProvider, useJobFeed } from "../context/JobFeedContext";
import { api } from "../hooks/useApi";

// The provider polls api.getRunState / api.getRunEvents as a safety net to
// settle jobs whose live "flow:event" stream went quiet. Mock both so tests
// can drive that reconciliation deterministically.
vi.mock("../hooks/useApi", () => ({
  api: {
    getRunState: vi.fn(),
    getRunEvents: vi.fn(),
  },
}));

// Helper to read context values.
function JobFeedReader({ onRead }: { onRead: (feed: ReturnType<typeof useJobFeed>) => void }) {
  const feed = useJobFeed();
  onRead(feed);
  return (
    <div>
      <span data-testid="job-count">{feed.jobs.length}</span>
      <span data-testid="has-active">{String(feed.hasActive)}</span>
      {feed.activeJob && <span data-testid="active-flow">{feed.activeJob.flowName}</span>}
      {feed.jobs.map((j) => (
        <span key={j.id} data-testid={`job-${j.id}`}>
          {j.status}
        </span>
      ))}
    </div>
  );
}

describe("JobFeedContext", () => {
  beforeEach(() => {
    // Default: backend still running, so the safety-net poll is a no-op and
    // jobs stay in whatever state the live path put them.
    vi.mocked(api.getRunState).mockResolvedValue("running");
    vi.mocked(api.getRunEvents).mockResolvedValue([]);
  });

  it("starts with empty state", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    expect(feed?.jobs).toHaveLength(0);
    expect(feed?.hasActive).toBe(false);
    expect(feed?.activeJob).toBeNull();
  });

  it("provides clearJob and clearAll functions", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    expect(typeof feed?.clearJob).toBe("function");
    expect(typeof feed?.clearAll).toBe("function");
  });

  it("shows no jobs initially", () => {
    render(
      <JobFeedProvider>
        <JobFeedReader onRead={() => {}} />
      </JobFeedProvider>,
    );

    expect(screen.getByTestId("job-count").textContent).toBe("0");
    expect(screen.getByTestId("has-active").textContent).toBe("false");
  });

  it("startJob initializes stepSnapshots as empty array", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    act(() => {
      feed?.startJob("test-flow", "project");
    });

    expect(feed?.jobs).toHaveLength(1);
    expect(feed?.jobs[0].stepSnapshots).toEqual([]);
    expect(feed?.jobs[0].status).toBe("running");
  });

  it("provides selectJob and startJob functions", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    expect(typeof feed?.selectJob).toBe("function");
    expect(typeof feed?.startJob).toBe("function");
  });

  it("settles a job stuck at running when the live complete event never arrives", async () => {
    // No "flow:event" is delivered (the live stream went quiet). The backend
    // reports the run finished, so the poll must reconcile the job to complete.
    vi.mocked(api.getRunState).mockResolvedValue("complete");
    vi.mocked(api.getRunEvents).mockResolvedValue([
      { type: "complete", flow_id: "test-flow", duration_ms: 4200 },
    ]);

    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    act(() => {
      feed?.startJob("test-flow", "project", undefined, 16);
    });
    expect(feed?.jobs[0].status).toBe("running");

    await waitFor(() => {
      expect(feed?.jobs[0].status).toBe("complete");
    });
    expect(feed?.hasActive).toBe(false);
    expect(feed?.jobs[0].durationMs).toBe(4200);
  });
});

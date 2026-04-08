import { render, screen, act } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { JobFeedProvider, useJobFeed, type Job } from "../context/JobFeedContext";

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
});

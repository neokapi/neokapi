import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "./testUtils";

// The page reads again on backend events, which no test runtime emits; the
// hook stands in and records what the page listens for.
const listened: Array<{ events: string[]; keys: readonly unknown[] }> = [];
vi.mock("../hooks/useInvalidateOnEvent", () => ({
  useInvalidateOnEvent: (events: string | string[], keys: readonly unknown[]) => {
    listened.push({ events: Array.isArray(events) ? events : [events], keys });
  },
}));

import { DecidePage } from "../components/DecidePage";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { DECIDE_VIEW, EMPTY_DECIDE_VIEW } from "@neokapi/project-screens/fixtures";

afterEach(() => {
  vi.restoreAllMocks();
  listened.length = 0;
});

describe("Decide wired to the backend", () => {
  it("reads the questions again when a decision is recorded, a file changes or the files are read again", () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<DecidePage tabID="t1" />);
    const decide = listened.find((l) =>
      l.keys.some((k) => JSON.stringify(k) === JSON.stringify(qk.rulesDecide("t1"))),
    );
    expect(decide?.events).toEqual(
      expect.arrayContaining(["workspace:changed", "project-files-changed", "project:extracted"]),
    );
  });

  it("moves the home's marker once it shows what waits", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    const mark = vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<DecidePage tabID="t1" />);
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    await waitFor(() => expect(mark).toHaveBeenCalledTimes(1));
  });

  it("leaves the marker where it is when nothing waits, so the news stays for History", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(EMPTY_DECIDE_VIEW);
    const mark = vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<DecidePage tabID="t1" />);
    await screen.findByText("Nothing to decide.");
    expect(mark).not.toHaveBeenCalled();
  });
});

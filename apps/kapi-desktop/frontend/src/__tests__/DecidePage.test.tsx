import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { render, fireEvent, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";

import { DECIDE_IDS, DECIDE_VIEW, WORD_VIEW } from "@neokapi/project-screens/fixtures";
import { DecidePage } from "../components/DecidePage";
import { api } from "../hooks/useApi";

afterEach(() => {
  vi.restoreAllMocks();
});

const selectedId = () =>
  document
    .querySelector<HTMLElement>("[data-slot='decide-item'][aria-selected='true']")
    ?.getAttribute("data-id");
const key = (k: string) => fireEvent.keyDown(window, { key: k });

describe("Decide wired to the backend", () => {
  it("opens the file a suggestion was seen in, and the Map, through the tab", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(WORD_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    const onOpenFile = vi.fn();
    render(<DecidePage tabID="t1" onOpenFile={onOpenFile} onOpenMap={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "about.html" }));
    expect(onOpenFile).toHaveBeenCalledWith("site/about.html");
  });

  it("shows the file without a link when the tab opens no file", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(WORD_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    render(<DecidePage tabID="t1" />);
    await screen.findByText("Keep this as a rule?");
    expect(document.querySelector("[data-slot='decide-open-file']")).toBeNull();
  });

  it("calls DecideRule for each answer on the wired page", async () => {
    vi.spyOn(api, "rulesDecideView").mockResolvedValue(DECIDE_VIEW);
    vi.spyOn(api, "markProjectContextDigestSeen").mockResolvedValue("");
    const decideRule = vi
      .spyOn(api, "decideRule")
      .mockResolvedValue({ answer: "keep", kept: 1, dropped: 1, later: 0 });
    render(<DecidePage tabID="t1" defaultSelected={`suggested:${DECIDE_IDS.quickcast}`} />);
    await waitFor(() => expect(selectedId()).toBe(DECIDE_IDS.quickcast));
    key("3");
    key("k");
    await waitFor(() =>
      expect(decideRule).toHaveBeenLastCalledWith("t1", DECIDE_IDS.quickcast, "keep", "QuickCast"),
    );
    await waitFor(() => expect(selectedId()).not.toBe(DECIDE_IDS.quickcast));
    key("n");
    await waitFor(() => expect(decideRule).toHaveBeenCalledTimes(2));
  });
});

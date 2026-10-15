import { afterEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor } from "./testUtils";

import { BOWRAIN_ABOUT_URL, SharePanel, shareRung, venueProject } from "../components/SharePanel";
import { api } from "../components/contextTypes";
import {
  BOWRAIN_CONNECTED,
  CONTACTED,
  FOLDER_UNREACHABLE,
  GIT_IN_SYNC,
  GIT_NEVER_SYNCED,
  GIT_PUSHES_WAITING,
  LOCAL_SHARING,
  SYNC_ERROR,
  SYNC_RESULT,
  WORKSPACE_DIR,
} from "../stories/fixtures/sharing";

afterEach(() => {
  vi.restoreAllMocks();
});

/** The rung element for one step of the ladder. */
const rung = (name: string) =>
  document.querySelector<HTMLElement>(`[data-slot='share-rung'][data-rung='${name}']`);

/** A rung's state line, as a reader reads it. */
const stateLine = (name: string) =>
  rung(name)?.querySelector("[data-slot='share-state']")?.textContent ?? "";

/** The rung the ladder marks. */
const marked = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='share-rung'][aria-current]")).map(
    (el) => el.dataset.rung,
  );

/** The last contact as the team rung draws it. */
const contactTime = () => {
  const time = rung("team")?.querySelector("time");
  expect(time).toHaveAttribute("dateTime", CONTACTED);
  return time?.textContent ?? "";
};

const NOT_CONNECTED =
  "Not connected. Bowrain holds every project's context in one workspace, with roles, review and a record of who decided what, and runs the project's loop on the server.";

describe("the rung a project stands on", () => {
  it("is the farthest it reaches", () => {
    expect(shareRung(LOCAL_SHARING)).toBe("mac");
    expect(shareRung(GIT_IN_SYNC)).toBe("team");
    expect(shareRung(BOWRAIN_CONNECTED)).toBe("bowrain");
  });

  it("names a Bowrain project by its URL's path", () => {
    expect(venueProject("https://app.bowrain.cloud/acme/help")).toBe("acme/help");
    expect(venueProject("https://app.bowrain.cloud/")).toBe("");
    expect(venueProject(undefined)).toBe("");
  });
});

describe("the Share ladder", () => {
  it("on this Mac: marks the first rung and offers Set up sharing", () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    expect(marked()).toEqual(["mac"]);
    expect(stateLine("mac")).toBe(
      `In your workspace at ${WORKSPACE_DIR}. Every clone and worktree of this project on this Mac reads it.`,
    );
    expect(stateLine("team")).toBe(
      "Not yet. Name a place and sync, and every machine that syncs the project reads the same context.",
    );
    expect(stateLine("bowrain")).toBe(NOT_CONNECTED);

    expect(screen.getByRole("button", { name: "Set up sharing" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sync now" })).toBeNull();
    expect(screen.getByRole("link", { name: "About Bowrain" })).toHaveAttribute(
      "href",
      BOWRAIN_ABOUT_URL,
    );
    expect(screen.getByText("kapi context sync")).toBeInTheDocument();
  });

  it("shared through git: marks the second rung, with the counts as of the last contact", () => {
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    expect(marked()).toEqual(["team"]);
    expect(stateLine("team")).toBe(
      `Through git (origin refs/kapi/context). 0 to push, 0 to pull, as of ${contactTime()}.`,
    );
    expect(screen.getByRole("button", { name: "Sync now" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Set up sharing" })).toBeNull();
  });

  it("shared, with pushes waiting", () => {
    render(<SharePanel tabID="t1" sharing={GIT_PUSHES_WAITING} />);
    expect(stateLine("team")).toBe(
      `Through git (origin refs/kapi/context). 2 to push, 0 to pull, as of ${contactTime()}.`,
    );
  });

  it("shared, and never synced from this Mac", () => {
    render(<SharePanel tabID="t1" sharing={GIT_NEVER_SYNCED} />);
    expect(stateLine("team")).toBe(
      "Through git (origin refs/kapi/context). 4 to push. What waits to pull is known after the first sync.",
    );
  });

  it("unreachable: says when the place could not be reached, and what it reported", () => {
    render(<SharePanel tabID="t1" sharing={FOLDER_UNREACHABLE} />);
    expect(stateLine("team")).toBe(
      `Through a folder (/Volumes/team/kapi/fernwell). The place could not be reached at ${contactTime()}.`,
    );
    expect(rung("team")?.querySelector("[data-slot='share-status-error']")?.textContent).toBe(
      FOLDER_UNREACHABLE.status?.error,
    );
    expect(screen.getByRole("button", { name: "Sync now" })).toBeInTheDocument();
  });

  it("connected to Bowrain: marks the third rung and names the server and project", () => {
    render(<SharePanel tabID="t1" sharing={BOWRAIN_CONNECTED} />);
    expect(marked()).toEqual(["bowrain"]);
    expect(stateLine("bowrain")).toBe("Connected to app.bowrain.cloud (acme/help).");
  });

  it("context-only: the first rung alone, and why sharing needs a checkout", () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} contextOnly />);
    expect(rung("mac")).not.toBeNull();
    expect(rung("team")).toBeNull();
    expect(rung("bowrain")).toBeNull();
    expect(document.querySelector("[data-slot='share-needs-checkout']")?.textContent).toBe(
      "Sharing is named in the project's kapi.yaml, so it needs a checkout of this project on this Mac.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("Set up sharing opens the dialog", async () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    await userEvent.click(screen.getByRole("button", { name: "Set up sharing" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Set up sharing");
  });
});

describe("Sync now", () => {
  it("syncs and shows what the sync did in the rung", async () => {
    const sync = vi.spyOn(api, "syncProjectContext").mockResolvedValue(SYNC_RESULT);
    render(<SharePanel tabID="t1" sharing={GIT_PUSHES_WAITING} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    expect(sync).toHaveBeenCalledWith("t1");
    await waitFor(() =>
      expect(rung("team")?.querySelector("[data-slot='share-sync-result']")?.textContent).toBe(
        "Pulled 3, pushed 2, rules files written",
      ),
    );
  });

  it("leaves out the rules files when the sync wrote none", async () => {
    vi.spyOn(api, "syncProjectContext").mockResolvedValue({
      ...SYNC_RESULT,
      rules_files: { files: [{ path: "AGENTS.md", action: "unchanged" }] },
    });
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));
    await waitFor(() =>
      expect(rung("team")?.querySelector("[data-slot='share-sync-result']")?.textContent).toBe(
        "Pulled 3, pushed 2",
      ),
    );
  });

  it("shows the error exactly as the backend returned it", async () => {
    vi.spyOn(api, "syncProjectContext").mockRejectedValue(new Error(SYNC_ERROR));
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    const error = await screen.findByRole("alert");
    expect(error.textContent).toBe(SYNC_ERROR);
    expect(rung("team")?.querySelector("[data-slot='share-sync-result']")).toBeNull();
  });
});

describe("reading the backend", () => {
  it("reads the project's sharing when none is given", async () => {
    const read = vi.spyOn(api, "projectSharing").mockResolvedValue(GIT_IN_SYNC);
    render(<SharePanel tabID="t1" />);
    expect(read).toHaveBeenCalledWith("t1");
    await waitFor(() => expect(marked()).toEqual(["team"]));
  });
});

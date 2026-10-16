import { afterEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import {
  BOWRAIN_ABOUT_URL,
  SharePanel,
  lastSegments,
  rungStates,
  shareRung,
  venueLink,
  venueParts,
  venueProject,
  type ShareRung,
} from "../components/SharePanel";
import { api } from "../hooks/useApi";
import {
  BOWRAIN_CONNECTED,
  BOWRAIN_WITHOUT_TEAM,
  CONTACTED,
  FOLDER_UNREACHABLE,
  GIT_IN_SYNC,
  GIT_NEVER_SYNCED,
  GIT_PUSHES_WAITING,
  LOCAL_SHARING,
  S3_IN_SYNC,
  SYNC_ERROR,
  SYNC_RESULT,
} from "../stories/fixtures/sharing";
import type { ProjectSharing } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

/** The stepper's element for one rung. */
const rung = (name: ShareRung) =>
  document.querySelector<HTMLElement>(`[data-slot='share-rung'][data-rung='${name}']`);

/** Each rung's state, as the stepper draws it. */
const states = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='share-rung']")).map(
    (el) => el.dataset.state,
  );

/** The rung the stepper marks as the one the project stands on. */
const marked = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='share-rung'][data-current]")).map(
    (el) => el.dataset.rung,
  );

const card = () => document.querySelector<HTMLElement>("[data-slot='share-card']")!;
const sentence = () => card().querySelector("[data-slot='share-state']")?.textContent ?? "";
const slot = (name: string) => card().querySelector<HTMLElement>(`[data-slot='${name}']`);

/** The facts in the card, as label and value. */
const facts = () => {
  const dts = Array.from(card().querySelectorAll("dt"));
  return Object.fromEntries(dts.map((dt) => [dt.textContent, dt.nextElementSibling?.textContent]));
};

/** The buttons drawn in the page's primary colour, anywhere in the panel. */
const primaries = () =>
  Array.from(
    document.querySelectorAll<HTMLElement>(
      "[data-slot='share-panel'] :is(button, a)[data-variant='default']",
    ),
  ).map((b) => b.textContent);

/** The last contact as the card draws it. */
const contactTime = () => {
  const time = card().querySelector("time");
  expect(time).toHaveAttribute("dateTime", CONTACTED);
  return time?.textContent ?? "";
};

const ABOUT =
  "Bowrain holds every project's context in one workspace, with roles, review and a record of who decided what, and runs the project's loop on the server.";

describe("the rung a project stands on", () => {
  it("is the farthest it reaches", () => {
    expect(shareRung(LOCAL_SHARING)).toBe("mac");
    expect(shareRung(GIT_IN_SYNC)).toBe("team");
    expect(shareRung(BOWRAIN_CONNECTED)).toBe("bowrain");
    expect(shareRung(BOWRAIN_WITHOUT_TEAM)).toBe("bowrain");
  });

  it("is current, with the reached rungs under it done and the rest to do", () => {
    expect(rungStates(LOCAL_SHARING)).toEqual({ mac: "current", team: "todo", bowrain: "todo" });
    expect(rungStates(GIT_IN_SYNC)).toEqual({ mac: "done", team: "current", bowrain: "todo" });
    expect(rungStates(BOWRAIN_CONNECTED)).toEqual({
      mac: "done",
      team: "done",
      bowrain: "current",
    });
    expect(rungStates(BOWRAIN_WITHOUT_TEAM)).toEqual({
      mac: "done",
      team: "todo",
      bowrain: "current",
    });
  });

  it("names a Bowrain project by its URL's path", () => {
    expect(venueProject("https://app.bowrain.cloud/acme/help")).toBe("acme/help");
    expect(venueProject("https://app.bowrain.cloud/")).toBe("");
    expect(venueProject(undefined)).toBe("");
    expect(venueParts("https://app.bowrain.cloud/acme/help")).toEqual({
      workspace: "acme",
      project: "help",
    });
    expect(venueParts("https://app.bowrain.cloud/projects/help")).toEqual({ project: "help" });
  });

  it("opens a Bowrain URL only over http or https, on the host the recipe names", () => {
    const host = "app.bowrain.cloud";
    expect(venueLink({ venue_url: "https://app.bowrain.cloud/acme/help", venue_host: host })).toBe(
      "https://app.bowrain.cloud/acme/help",
    );
    expect(
      venueLink({ venue_url: "http://localhost:8080/acme/help", venue_host: "localhost:8080" }),
    ).toBe("http://localhost:8080/acme/help");
    expect(venueLink({ venue_url: "javascript:alert(1)", venue_host: host })).toBeNull();
    expect(venueLink({ venue_url: "file:///etc/passwd", venue_host: host })).toBeNull();
    expect(venueLink({ venue_url: "data:text/html,<p>hi</p>", venue_host: host })).toBeNull();
    expect(
      venueLink({ venue_url: "https://elsewhere.example/acme/help", venue_host: host }),
    ).toBeNull();
    expect(venueLink({ venue_url: "not a url", venue_host: host })).toBeNull();
    expect(venueLink({ venue_url: undefined, venue_host: host })).toBeNull();
  });

  it("shows a path by its last segments", () => {
    expect(lastSegments("/fakehome/.local/share/kapi/workspaces/default", 2)).toBe(
      "workspaces/default",
    );
    expect(lastSegments("C:\\Users\\me\\kapi\\workspaces\\default\\", 2)).toBe(
      "workspaces/default",
    );
  });
});

describe("the Share page", () => {
  it("has its title and subtitle, and no command line", () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Share");
    expect(screen.getByText("How far this project's context reaches.")).toBeInTheDocument();
    expect(screen.queryByText(/kapi context sync/)).toBeNull();
    expect(screen.queryByText(/^Not yet/)).toBeNull();
    expect(screen.queryByText(/^Not connected/)).toBeNull();
  });

  it("draws the three rungs as a stepper", () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    const ladder = screen.getByRole("radiogroup", { name: "How far this context reaches" });
    expect(ladder.dataset.slot).toBe("share-ladder");
    expect(
      within(ladder)
        .getAllByRole("radio")
        .map((r) => r.textContent),
    ).toEqual(["On this Mac", "Shared with your team", "Managed on Bowrain"]);
  });

  it("on this Mac: the first rung current, its card and Set up sharing", () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    expect(marked()).toEqual(["mac"]);
    expect(rung("mac")).toHaveAttribute("aria-current", "step");
    expect(states()).toEqual(["current", "todo", "todo"]);

    expect(card().dataset.rung).toBe("mac");
    expect(within(card()).getByRole("heading", { level: 2 })).toHaveTextContent("On this Mac");
    expect(sentence()).toBe(
      "Every clone and worktree of this project on this Mac reads the same context.",
    );
    expect(slot("share-workspace")?.textContent).toBe("workspaces/default");
    expect(primaries()).toEqual(["Set up sharing"]);
  });

  it("shared through git: the second rung current, with the place and the last sync", () => {
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    expect(marked()).toEqual(["team"]);
    expect(states()).toEqual(["done", "current", "todo"]);
    expect(card().dataset.rung).toBe("team");
    expect(slot("share-kind")?.textContent).toBe("git");
    expect(slot("share-place")?.textContent).toBe("origin refs/kapi/context");
    expect(facts()).toEqual({
      Through: "gitorigin refs/kapi/context",
      "Last sync": contactTime(),
      "To push": "0",
      "To pull": "0",
    });
    expect(primaries()).toEqual(["Sync now"]);
    expect(within(card()).getByRole("button", { name: "Change" })).toHaveAttribute(
      "data-variant",
      "ghost",
    );
  });

  it("shared, with pushes waiting", () => {
    render(<SharePanel tabID="t1" sharing={GIT_PUSHES_WAITING} />);
    expect(facts()["To push"]).toBe("2");
    expect(facts()["To pull"]).toBe("0");
  });

  it("shared, and never synced from this Mac", () => {
    render(<SharePanel tabID="t1" sharing={GIT_NEVER_SYNCED} />);
    expect(facts()).toEqual({
      Through: "gitorigin refs/kapi/context",
      "Last sync": "Not yet from this Mac",
      "To push": "4",
      "To pull": "Known after the first sync",
    });
  });

  it("through a folder that could not be reached: says when, and what it reported", () => {
    render(<SharePanel tabID="t1" sharing={FOLDER_UNREACHABLE} />);
    expect(slot("share-kind")?.textContent).toBe("A folder");
    expect(slot("share-place")?.textContent).toBe("/Volumes/team/kapi/fernwell");
    expect(screen.getByText(/^The place could not be reached at/).textContent).toBe(
      `The place could not be reached at ${contactTime()}.`,
    );
    expect(slot("share-status-error")?.textContent).toBe(FOLDER_UNREACHABLE.status?.error);
    expect(primaries()).toEqual(["Sync now"]);
  });

  it("through S3", () => {
    render(<SharePanel tabID="t1" sharing={S3_IN_SYNC} />);
    expect(slot("share-kind")?.textContent).toBe("S3");
    expect(slot("share-place")?.textContent).toBe("s3://acme-kapi/fernwell/");
  });

  it("connected to Bowrain: the third rung current, with the server and workspace", () => {
    render(<SharePanel tabID="t1" sharing={BOWRAIN_CONNECTED} />);
    expect(marked()).toEqual(["bowrain"]);
    expect(states()).toEqual(["done", "done", "current"]);
    expect(card().dataset.rung).toBe("bowrain");
    expect(sentence()).toBe("The recipe connects this project to a Bowrain server.");
    expect(slot("share-server")?.textContent).toBe("app.bowrain.cloud");
    expect(slot("share-venue-workspace")?.textContent).toBe("acme");
    const open = within(card()).getByRole("link", { name: "Open the project" });
    expect(open).toHaveAttribute("href", "https://app.bowrain.cloud/acme/help");
    expect(open).toHaveAttribute("target", "_blank");
    expect(open).toHaveAttribute("rel", "noopener noreferrer");
    expect(primaries()).toEqual([]);
  });

  it("connected to Bowrain through a URL with another scheme: offers no link to open", () => {
    render(
      <SharePanel
        tabID="t1"
        sharing={{ ...BOWRAIN_CONNECTED, venue_url: "javascript:alert(document.domain)" }}
      />,
    );
    expect(card().dataset.rung).toBe("bowrain");
    expect(slot("share-server")?.textContent).toBe("app.bowrain.cloud");
    expect(slot("share-open-bowrain")).toBeNull();
    expect(within(card()).queryByRole("link")).toBeNull();
  });

  it("connected to Bowrain with no team place: the team rung stays to do", () => {
    render(<SharePanel tabID="t1" sharing={BOWRAIN_WITHOUT_TEAM} />);
    expect(states()).toEqual(["done", "todo", "current"]);
  });

  it("context-only: the rungs, and why sharing needs a checkout, with no button", async () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} contextOnly />);
    expect(states()).toEqual(["current", "todo", "todo"]);
    const needs =
      "Sharing is named in the project's kapi.yaml, so it needs a checkout of this project on this Mac.";
    expect(slot("share-needs-checkout")?.textContent).toBe(needs);
    expect(within(card()).queryByRole("button")).toBeNull();

    await userEvent.click(rung("team")!);
    expect(card().dataset.rung).toBe("team");
    expect(slot("share-needs-checkout")?.textContent).toBe(needs);
    expect(within(card()).queryByRole("button")).toBeNull();
  });
});

describe("choosing a rung to read about", () => {
  it("shows the clicked rung's card and keeps the current rung marked", async () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    await userEvent.click(rung("bowrain")!);
    expect(card().dataset.rung).toBe("bowrain");
    expect(rung("bowrain")).toHaveAttribute("aria-checked", "true");
    expect(marked()).toEqual(["mac"]);
    expect(sentence()).toBe(ABOUT);
    const about = within(card()).getByRole("link", { name: "About Bowrain" });
    expect(about).toHaveAttribute("href", BOWRAIN_ABOUT_URL);
    // Both links on the page open the way every external link in the app does.
    expect(about).toHaveAttribute("target", "_blank");
    expect(about).toHaveAttribute("rel", "noopener noreferrer");

    await userEvent.click(rung("team")!);
    expect(card().dataset.rung).toBe("team");
    expect(sentence()).toBe(
      "Every machine that syncs this project reads the same context, through a git ref, a folder or an S3 bucket.",
    );
    expect(primaries()).toEqual(["Set up sharing"]);
  });

  it("moves between rungs with the arrow keys", async () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    rung("mac")!.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(card().dataset.rung).toBe("team");
    await userEvent.keyboard("{ArrowRight}");
    expect(card().dataset.rung).toBe("bowrain");
  });

  it("offers at most one primary button on every rung, and none on Bowrain's", async () => {
    const expected: Array<[ProjectSharing, Record<ShareRung, string[]>]> = [
      [LOCAL_SHARING, { mac: ["Set up sharing"], team: ["Set up sharing"], bowrain: [] }],
      [GIT_IN_SYNC, { mac: [], team: ["Sync now"], bowrain: [] }],
      [BOWRAIN_CONNECTED, { mac: [], team: ["Sync now"], bowrain: [] }],
    ];
    for (const [sharing, perRung] of expected) {
      const { unmount } = render(<SharePanel tabID="t1" sharing={sharing} />);
      for (const r of ["mac", "team", "bowrain"] as const) {
        await userEvent.click(rung(r)!);
        expect({ rung: r, primaries: primaries() }).toEqual({ rung: r, primaries: perRung[r] });
      }
      unmount();
    }
  });
});

describe("setting up and changing the place", () => {
  it("Set up sharing opens the dialog", async () => {
    render(<SharePanel tabID="t1" sharing={LOCAL_SHARING} />);
    await userEvent.click(screen.getByRole("button", { name: "Set up sharing" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Set up sharing");
  });

  it("Change opens the same dialog", async () => {
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Set up sharing");
  });
});

describe("Sync now", () => {
  it("syncs and shows what the sync did in the card", async () => {
    const sync = vi.spyOn(api, "syncProjectContext").mockResolvedValue(SYNC_RESULT);
    render(<SharePanel tabID="t1" sharing={GIT_PUSHES_WAITING} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    expect(sync).toHaveBeenCalledWith("t1");
    await waitFor(() =>
      expect(slot("share-sync-result")?.textContent).toBe(
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
    await waitFor(() => expect(slot("share-sync-result")?.textContent).toBe("Pulled 3, pushed 2"));
  });

  it("shows the error exactly as the backend returned it", async () => {
    vi.spyOn(api, "syncProjectContext").mockRejectedValue(new Error(SYNC_ERROR));
    render(<SharePanel tabID="t1" sharing={GIT_IN_SYNC} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    const error = await screen.findByRole("alert");
    expect(error.textContent).toBe(SYNC_ERROR);
    expect(slot("share-sync-result")).toBeNull();
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

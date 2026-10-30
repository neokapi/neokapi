import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";

import {
  BOWRAIN_ABOUT_URL,
  BOWRAIN_SHARE_COMMAND,
  SharePanel,
  lastSegments,
  reachedRungs,
  venueLink,
  venueParts,
  venueProject,
} from "../components/SharePanel";
import { api } from "../hooks/useApi";
import {
  BOWRAIN_VIEW,
  BOWRAIN_WITHOUT_TEAM_VIEW,
  CONTACTED,
  FOLDER_UNREACHABLE,
  FOLDER_UNREACHABLE_VIEW,
  GIT_NEVER_SYNCED_VIEW,
  GIT_PUSHES_WAITING_VIEW,
  GIT_VIEW,
  LOCAL_VIEW,
  LOCAL_VIEW_WITH_PLUGIN,
  S3_VIEW,
  SYNC_ERROR,
  SYNC_RESULT,
  UNSAVED_VIEW,
} from "../stories/fixtures/sharing";
import type { ShareRungID } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

/** The list item for one rung. */
const rung = (name: ShareRungID) =>
  document.querySelector<HTMLElement>(`[data-slot='share-rung'][data-rung='${name}']`)!;

/** The rungs, nearest first, by name. */
const names = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='share-rung']")).map(
    (el) => el.querySelector("p")?.textContent,
  );

/** The rung the list marks as the one the project stands on. */
const marked = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='share-rung'][data-current]")).map(
    (el) => el.dataset.rung,
  );

const reach = (name: ShareRungID) =>
  rung(name).querySelector("[data-slot='share-reach']")?.textContent ?? "";
const slot = (name: string, within: HTMLElement = document.body) =>
  within.querySelector<HTMLElement>(`[data-slot='${name}']`);

/** The facts under a rung, as label and value. */
const facts = (name: ShareRungID) => {
  const dts = Array.from(rung(name).querySelectorAll("dt"));
  return Object.fromEntries(dts.map((dt) => [dt.textContent, dt.nextElementSibling?.textContent]));
};

/** The buttons drawn in the page's primary colour, anywhere in the panel. */
const primaries = () =>
  Array.from(
    document.querySelectorAll<HTMLElement>(
      "[data-slot='share-panel'] :is(button, a)[data-variant='default']",
    ),
  ).map((b) => b.textContent);

/** The last contact as the team rung draws it. */
const contactTime = () => {
  const time = rung("team").querySelector("time");
  expect(time).toHaveAttribute("dateTime", CONTACTED);
  return time?.textContent ?? "";
};

describe("the rungs a project has reached", () => {
  it("are this Mac always, the team's once a place is named, and Bowrain's once connected", () => {
    expect(reachedRungs(LOCAL_VIEW)).toEqual({ machine: true, team: false, bowrain: false });
    expect(reachedRungs(GIT_VIEW)).toEqual({ machine: true, team: true, bowrain: false });
    expect(reachedRungs(BOWRAIN_VIEW)).toEqual({ machine: true, team: true, bowrain: true });
    expect(reachedRungs(BOWRAIN_WITHOUT_TEAM_VIEW)).toEqual({
      machine: true,
      team: false,
      bowrain: true,
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
  it("is titled with the project, says how far the rules reach, and lists three rungs", () => {
    render(<SharePanel tabID="t1" projectName="Fernwell" view={LOCAL_VIEW} />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Fernwell · Share");
    expect(screen.getByText("How far your rules reach", { selector: "p" })).toBeInTheDocument();
    const ladder = screen.getByRole("list", { name: "How far your rules reach" });
    expect(ladder.dataset.slot).toBe("share-ladder");
    expect(within(ladder).getAllByRole("listitem")).toHaveLength(3);
    expect(names()).toEqual(["On this Mac", "With your team", "Managed on Bowrain"]);
    expect(screen.queryByText(/kapi context sync/)).toBeNull();
  });

  it("names what each rung changes for the person", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} />);
    expect(reach("machine")).toBe("Your rules, your assistants");
    expect(reach("team")).toBe("A shared folder or git");
    expect(reach("bowrain")).toBe("A team queue, who kept what, the same screens in a browser");
  });

  it("on this Mac: the first rung current, with the workspace and Change", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} />);
    expect(marked()).toEqual(["machine"]);
    expect(rung("machine")).toHaveAttribute("aria-current", "step");
    expect(slot("share-current", rung("machine"))?.textContent).toBe("Current");
    expect(slot("share-workspace", rung("machine"))?.textContent).toBe("workspaces/default");
    expect(within(rung("machine")).getByRole("button", { name: "Change" })).toBeInTheDocument();
    expect(within(rung("team")).queryByRole("button")).toBeNull();
    expect(primaries()).toEqual(["Change"]);
  });

  it("in a folder opened without a kapi.yaml: the same first rung, with Change", () => {
    render(<SharePanel tabID="t1" view={UNSAVED_VIEW} />);
    expect(marked()).toEqual(["machine"]);
    expect(primaries()).toEqual(["Change"]);
  });

  it("with your team through git: the place, the rules, the last sync and the counts", () => {
    render(<SharePanel tabID="t1" view={GIT_VIEW} />);
    expect(marked()).toEqual(["team"]);
    expect(rung("team")).toHaveAttribute("aria-current", "step");
    expect(reach("team")).toBe(`12 rules · synced ${contactTime()}`);
    expect(slot("share-kind", rung("team"))?.textContent).toBe("git");
    expect(slot("share-place", rung("team"))?.textContent).toBe("origin refs/kapi/context");
    expect(facts("team")).toEqual({
      Through: "gitorigin refs/kapi/context",
      "To push": "0",
      "To pull": "0",
    });
    // The workspace belongs to the first rung only while it is current.
    expect(slot("share-workspace")).toBeNull();
    expect(primaries()).toEqual(["Sync now"]);
    expect(within(rung("team")).getByRole("button", { name: "Change" })).toHaveAttribute(
      "data-variant",
      "ghost",
    );
  });

  it("with pushes waiting", () => {
    render(<SharePanel tabID="t1" view={GIT_PUSHES_WAITING_VIEW} />);
    expect(facts("team")["To push"]).toBe("2");
    expect(facts("team")["To pull"]).toBe("0");
  });

  it("never synced from this Mac", () => {
    render(<SharePanel tabID="t1" view={GIT_NEVER_SYNCED_VIEW} />);
    expect(reach("team")).toBe("12 rules · not synced from this Mac yet");
    expect(facts("team")).toEqual({
      Through: "gitorigin refs/kapi/context",
      "To push": "4",
      "To pull": "Known after the first sync",
    });
  });

  it("one rule, and rules that could not be counted", () => {
    const { unmount } = render(<SharePanel tabID="t1" view={{ ...GIT_VIEW, rules: 1 }} />);
    expect(reach("team")).toBe(`1 rule · synced ${contactTime()}`);
    unmount();
    render(<SharePanel tabID="t1" view={{ ...GIT_VIEW, rules: undefined }} />);
    expect(reach("team")).toBe(`synced ${contactTime()}`);
  });

  it("through a folder that could not be reached: says when, and what it reported", () => {
    render(<SharePanel tabID="t1" view={FOLDER_UNREACHABLE_VIEW} />);
    expect(slot("share-kind", rung("team"))?.textContent).toBe("A shared folder");
    expect(slot("share-place", rung("team"))?.textContent).toBe("/Volumes/team/kapi/fernwell");
    expect(screen.getByText(/^The place could not be reached at/).textContent).toBe(
      `The place could not be reached at ${contactTime()}.`,
    );
    expect(slot("share-status-error")?.textContent).toBe(FOLDER_UNREACHABLE.status?.error);
    expect(primaries()).toEqual(["Sync now"]);
  });

  it("through S3", () => {
    render(<SharePanel tabID="t1" view={S3_VIEW} />);
    expect(slot("share-kind", rung("team"))?.textContent).toBe("S3");
    expect(slot("share-place", rung("team"))?.textContent).toBe("s3://acme-kapi/fernwell/");
  });

  it("before Bowrain: the command that connects it and About Bowrain, with no button to press", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} />);
    const bowrain = rung("bowrain");
    expect(slot("share-bowrain-command", bowrain)?.textContent).toBe(BOWRAIN_SHARE_COMMAND);
    expect(slot("share-bowrain-plugin", bowrain)?.textContent).toBe(
      "Needs the kapi-bowrain plugin",
    );
    const about = within(bowrain).getByRole("link", { name: "About Bowrain" });
    expect(about).toHaveAttribute("href", BOWRAIN_ABOUT_URL);
    expect(about).toHaveAttribute("target", "_blank");
    expect(about).toHaveAttribute("rel", "noopener noreferrer");
    expect(within(bowrain).queryByRole("button")).toBeNull();
  });

  it("before Bowrain, with the plugin installed: the command alone", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW_WITH_PLUGIN} />);
    expect(slot("share-bowrain-command", rung("bowrain"))).not.toBeNull();
    expect(slot("share-bowrain-plugin", rung("bowrain"))).toBeNull();
  });

  it("managed on Bowrain: the third rung current, with the server, and the team place under its own", () => {
    render(<SharePanel tabID="t1" view={BOWRAIN_VIEW} />);
    expect(marked()).toEqual(["bowrain"]);
    const bowrain = rung("bowrain");
    expect(slot("share-server", bowrain)?.textContent).toBe("app.bowrain.cloud");
    expect(slot("share-venue-workspace", bowrain)?.textContent).toBe("acme");
    expect(slot("share-bowrain-command")).toBeNull();
    const open = within(bowrain).getByRole("link", { name: "Open the project" });
    expect(open).toHaveAttribute("href", "https://app.bowrain.cloud/acme/help");
    expect(open).toHaveAttribute("target", "_blank");
    expect(open).toHaveAttribute("rel", "noopener noreferrer");
    expect(slot("share-place", rung("team"))?.textContent).toBe("origin refs/kapi/context");
    expect(primaries()).toEqual(["Sync now"]);
  });

  it("managed on Bowrain through a URL with another scheme: offers no link to open", () => {
    render(
      <SharePanel
        tabID="t1"
        view={{
          ...BOWRAIN_VIEW,
          sharing: { ...BOWRAIN_VIEW.sharing, venue_url: "javascript:alert(document.domain)" },
        }}
      />,
    );
    expect(slot("share-server", rung("bowrain"))?.textContent).toBe("app.bowrain.cloud");
    expect(slot("share-open-bowrain")).toBeNull();
    expect(within(rung("bowrain")).queryByRole("link")).toBeNull();
  });

  it("managed on Bowrain with no team place: the team rung offers Change", () => {
    render(<SharePanel tabID="t1" view={BOWRAIN_WITHOUT_TEAM_VIEW} />);
    expect(marked()).toEqual(["bowrain"]);
    expect(reach("team")).toBe("A shared folder or git");
    expect(within(rung("team")).getByRole("button", { name: "Change" })).toHaveAttribute(
      "data-variant",
      "outline",
    );
    expect(primaries()).toEqual([]);
  });

  it("context-only: the rungs, and why sharing needs a checkout, with no button", () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} contextOnly />);
    expect(marked()).toEqual(["machine"]);
    expect(slot("share-needs-checkout")?.textContent).toBe(
      "Sharing is named in the project's kapi.yaml, so it needs a checkout of this project on this Mac.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("Change", () => {
  it("opens the dialog from this Mac's rung", async () => {
    render(<SharePanel tabID="t1" view={LOCAL_VIEW} />);
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Set up sharing");
  });

  it("opens the same dialog from the team's rung", async () => {
    render(<SharePanel tabID="t1" view={GIT_VIEW} />);
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Set up sharing");
  });
});

describe("Sync now", () => {
  it("calls the sync binding and shows what the sync did under the rung", async () => {
    const sync = vi.spyOn(api, "syncProjectContext").mockResolvedValue(SYNC_RESULT);
    render(<SharePanel tabID="t1" view={GIT_PUSHES_WAITING_VIEW} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    expect(sync).toHaveBeenCalledWith("t1");
    await waitFor(() =>
      expect(slot("share-sync-result", rung("team"))?.textContent).toBe(
        "Pulled 3, pushed 2, rules files written",
      ),
    );
  });

  it("leaves out the rules files when the sync wrote none", async () => {
    vi.spyOn(api, "syncProjectContext").mockResolvedValue({
      ...SYNC_RESULT,
      rules_files: { files: [{ path: "AGENTS.md", action: "unchanged" }] },
    });
    render(<SharePanel tabID="t1" view={GIT_VIEW} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));
    await waitFor(() => expect(slot("share-sync-result")?.textContent).toBe("Pulled 3, pushed 2"));
  });

  it("shows the error exactly as the backend returned it", async () => {
    vi.spyOn(api, "syncProjectContext").mockRejectedValue(new Error(SYNC_ERROR));
    render(<SharePanel tabID="t1" view={GIT_VIEW} />);
    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));

    const error = await screen.findByRole("alert");
    expect(error.textContent).toBe(SYNC_ERROR);
    expect(slot("share-sync-result")).toBeNull();
  });
});

describe("reading the backend", () => {
  it("reads the Share page when none is given", async () => {
    const read = vi.spyOn(api, "shareView").mockResolvedValue(GIT_VIEW);
    render(<SharePanel tabID="t1" />);
    expect(read).toHaveBeenCalledWith("t1");
    await waitFor(() => expect(marked()).toEqual(["team"]));
  });
});

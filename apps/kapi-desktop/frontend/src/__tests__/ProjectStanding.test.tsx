import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ProjectStanding, type ProjectPointsResult } from "../components/ProjectStanding";
import type { KapiProject, ProjectStatus } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Northsea",
  defaults: { source_language: "en-US", coordinates: { brand: "northsea" } },
  collections: [{ name: "Docs", content: [{ path: "docs/**/*.md" }] }],
};

const status: ProjectStatus = {
  projectPath: "/p/northsea/kapi.yaml",
  projectName: "Northsea",
  hasData: true,
  collections: [
    { name: "Docs", blockCount: 12, coverage: {}, targetLanguages: [] },
    { name: "App", blockCount: 30, coverage: {}, targetLanguages: [] },
  ],
};

const points: ProjectPointsResult = {
  at: "2026-08-30T09:00:00Z",
  points: [
    {
      ref: "",
      label: "project default",
      default: true,
      coordinates: { brand: "northsea" },
      collections: ["App"],
      voice: "Northsea",
      voice_field: "defaults.voice",
      termstore: ".kapi/terms.json",
    },
    {
      ref: "support/docs",
      label: "support/docs",
      profile: "support",
      channel: "docs",
      default: false,
      coordinates: { brand: "northsea", product: "support", channel: "docs" },
      collections: ["Docs"],
      voice: "Northsea Support",
      voice_field: "profiles.support.voice",
    },
    {
      ref: "campaign/promo",
      label: "campaign/promo",
      profile: "campaign",
      channel: "promo",
      default: false,
      collections: [],
      voice: "Northsea",
      fallback: {
        profile: "campaign",
        expired: true,
        boundary: "2026-08-29T00:00:00Z",
        message: 'profile "campaign" expired 2026-08-29',
      },
    },
  ],
};

describe("ProjectStanding", () => {
  it("states both axes in the CLI's own shape", () => {
    render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: false, stream: "main" }}
      />,
    );
    const standing = screen.getByTestId("project-standing");
    expect(standing).toHaveTextContent("stream main");
    expect(standing).toHaveTextContent("42 unit(s) extracted");
    expect(standing).toHaveTextContent("2 collection(s)");
    expect(screen.getByTestId("standing-voice")).toHaveTextContent("voice Northsea");
    expect(standing).toHaveTextContent(".kapi/terms.json");
    expect(standing).toHaveTextContent("3 of 3 point(s) governed");
  });

  it("names the venue when one is connected", () => {
    render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: true, host: "app.bowrain.cloud", stream: "release-2" }}
      />,
    );
    const standing = screen.getByTestId("project-standing");
    expect(standing).toHaveTextContent("venue app.bowrain.cloud");
    expect(standing).toHaveTextContent("stream release-2");
  });

  it("says nothing is extracted rather than showing a zero", () => {
    render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={{ ...status, hasData: false }}
        points={points}
        server={{ connected: false }}
      />,
    );
    expect(screen.getByTestId("project-standing")).toHaveTextContent("nothing extracted yet");
  });

  it("lists every declared point with its collections and voice", () => {
    render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: false }}
      />,
    );
    const rows = screen.getAllByTestId("point-row");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("project default");
    expect(
      rows[0].querySelector("[data-slot='coordinate-chip'][data-axis='brand']"),
    ).toHaveTextContent("northsea");
    expect(rows[1]).toHaveTextContent("support/docs");
    expect(rows[1]).toHaveTextContent("Docs");
    expect(rows[1]).toHaveTextContent("Northsea Support");
    // A point nothing sits at says so rather than reading as an error.
    expect(rows[2]).toHaveTextContent("no collection sits here");
    expect(rows[2]).toHaveTextContent("fell through");
  });

  it("opens Context standing at the point a row names", async () => {
    const onOpenPoint = vi.fn();
    render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: false }}
        onOpenPoint={onOpenPoint}
      />,
    );
    await userEvent.click(screen.getAllByTestId("point-row")[1]);
    expect(onOpenPoint).toHaveBeenCalledWith({ coordinate: "support/docs", collection: "Docs" });
  });

  // R15.4: a single-point project gets one quiet row, not a hidden map.
  it("shows one row for a project that declares a single point", () => {
    render(
      <ProjectStanding
        tabID="t1"
        project={{ version: "v1", name: "Solo" }}
        displayName="Solo"
        status={{ ...status, collections: [] }}
        points={{ at: points.at, points: [points.points[0]] }}
        server={{ connected: false }}
      />,
    );
    const map = screen.getByTestId("point-map");
    expect(within(map).getAllByTestId("point-row")).toHaveLength(1);
    expect(map).toHaveTextContent("project default");
  });

  it("keeps the languages axis quiet until the project declares targets", () => {
    const { rerender } = render(
      <ProjectStanding
        tabID="t1"
        project={project}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: false }}
      />,
    );
    expect(screen.getByTestId("project-standing")).not.toHaveTextContent("languages");

    rerender(
      <ProjectStanding
        tabID="t1"
        project={{ ...project, defaults: { ...project.defaults, target_languages: ["nb-NO"] } }}
        displayName="Northsea"
        status={status}
        points={points}
        server={{ connected: false }}
      />,
    );
    expect(screen.getByTestId("project-standing")).toHaveTextContent("nb-NO");
  });
});

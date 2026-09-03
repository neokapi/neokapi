import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { GovernanceSettings, type RecipeGovernance } from "../components/GovernanceSettings";
import type { KapiProject } from "../types/api";

const governance: RecipeGovernance = {
  axes: [
    {
      axis: "product",
      declarable: false,
      refusal:
        "recipe: \"product\" is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead",
    },
    {
      axis: "channel",
      declarable: false,
      refusal:
        "recipe: \"channel\" is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead",
    },
    { axis: "brand", declarable: true, used: "northsea" },
    {
      axis: "mode",
      declarable: true,
      values: ["tutorial", "how-to", "reference", "explanation"],
    },
  ],
  channels: ["campaign/promo", "support/docs"],
  profiles: ["campaign", "support"],
  voice_files: [".kapi/voice.yaml", ".kapi/profiles/support/voice.yaml"],
  packs: ["technical-docs", "friendly-dtc"],
};

const project: KapiProject = {
  version: "v1",
  name: "Northsea",
  defaults: {
    source_language: "en-US",
    coordinates: { brand: "northsea" },
    voice: { profile_file: ".kapi/voice.yaml" },
  },
  flows: { translate: { steps: [] } },
};

function renderSettings(overrides: Partial<KapiProject> = {}) {
  const onUpdate = vi.fn();
  render(
    <GovernanceSettings
      tabID="t1"
      project={{ ...project, ...overrides }}
      onUpdate={onUpdate}
      governance={governance}
    />,
  );
  return onUpdate;
}

describe("GovernanceSettings", () => {
  it("shows the bound voice profile", () => {
    renderSettings();
    expect(screen.getByLabelText("Voice profile")).toHaveTextContent(".kapi/voice.yaml");
  });

  it("binds a starter pack", async () => {
    const onUpdate = renderSettings();
    await userEvent.click(screen.getByLabelText("Voice profile"));
    await userEvent.click(screen.getByRole("option", { name: /technical-docs/ }));
    expect(onUpdate.mock.calls[0][0].defaults.voice).toEqual({ pack: "technical-docs" });
  });

  it("edits a declared axis", async () => {
    const onUpdate = renderSettings();
    const value = within(screen.getByTestId("coordinates-editor")).getByLabelText("brand");
    await userEvent.clear(value);
    expect(onUpdate).toHaveBeenCalled();
    expect(onUpdate.mock.calls[0][0].defaults.coordinates).toEqual({ brand: "" });
  });

  it("removes a declared axis, and drops the key when it was the last", async () => {
    const onUpdate = renderSettings();
    await userEvent.click(screen.getByRole("button", { name: "Remove brand" }));
    expect(onUpdate.mock.calls[0][0].defaults.coordinates).toBeUndefined();
  });

  it("refuses a structural axis with the recipe's own words", async () => {
    renderSettings();
    await userEvent.type(screen.getByLabelText("New axis"), "product");
    expect(screen.getByTestId("axis-refusal")).toHaveTextContent(
      "derived from a collection's channel",
    );
    expect(screen.getByRole("button", { name: /Add axis/ })).toBeDisabled();
  });

  it("adds a declarable axis", async () => {
    const onUpdate = renderSettings();
    await userEvent.type(screen.getByLabelText("New axis"), "mode");
    await userEvent.click(screen.getByRole("button", { name: /Add axis/ }));
    expect(onUpdate.mock.calls[0][0].defaults.coordinates).toEqual({ brand: "northsea", mode: "" });
  });

  it("splits governance into where content sits and what runs", () => {
    renderSettings();
    expect(screen.getByText("Where content sits")).toBeInTheDocument();
    expect(screen.getByText("What runs, and what is skipped")).toBeInTheDocument();
    // The channel map (its own data source) replaces the old declared-channels
    // list; the names themselves are covered by the ChannelMap tests.
    expect(screen.getByTestId("channel-map")).toBeInTheDocument();
    expect(screen.queryByText("Declared channels")).not.toBeInTheDocument();
  });

  it("edits the default flow", async () => {
    const onUpdate = renderSettings();
    await userEvent.click(screen.getByLabelText("Default flow"));
    await userEvent.click(screen.getByRole("option", { name: "translate" }));
    expect(onUpdate.mock.calls[0][0].defaults.flow).toBe("translate");
  });

  it("edits the excluded paths", async () => {
    const onUpdate = renderSettings();
    const input = screen.getByPlaceholderText("Add a pattern, e.g. **/vendor/**");
    await userEvent.type(input, "**/vendor/**{Enter}");
    expect(onUpdate.mock.calls.at(-1)?.[0].defaults.exclude).toEqual(["**/vendor/**"]);
  });

  it("reads as complete for a project that declares nothing", () => {
    render(
      <GovernanceSettings
        tabID="t1"
        project={{ version: "v1", name: "Solo" }}
        onUpdate={vi.fn()}
        governance={{
          axes: governance.axes,
          channels: [],
          profiles: [],
          voice_files: [],
          packs: [],
        }}
      />,
    );
    expect(
      screen.getByText("Every collection sits at the project's own point."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Declared channels")).not.toBeInTheDocument();
  });
});

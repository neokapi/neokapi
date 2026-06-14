// @vitest-environment jsdom
// Smoke test for the horizontal EvolutionRoadmap renderer (Apache-2.0). It
// builds a real model from a small inline concept — an English directory→folder
// rename, a German term from genesis, and a Norwegian sibling joining later — so
// the test exercises lanes, a rename milestone, and a sibling branch through the
// actual builder, then asserts the roadmap mounts and surfaces a locale label.

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EvolutionRoadmap } from "./EvolutionRoadmap";
import { buildEvolutionModel } from "./evolution-model";
import type { EvolutionInput } from "./evolution-model";
import type { Concept } from "./types";

const NOW = "2026-06-14T00:00:00.000Z";

const concept: Concept = {
  id: "c-folder",
  domain: "product",
  createdAt: "2023-01-01T00:00:00.000Z",
  updatedAt: "2024-06-01T00:00:00.000Z",
  terms: [
    {
      text: "directory",
      locale: "en",
      status: "deprecated",
      validity: {
        validFrom: "2023-01-01T00:00:00.000Z",
        validTo: "2024-06-01T00:00:00.000Z",
      },
    },
    {
      text: "folder",
      locale: "en",
      status: "preferred",
      validity: { validFrom: "2024-06-01T00:00:00.000Z" },
    },
    {
      text: "Ordner",
      locale: "de",
      status: "approved",
      validity: { validFrom: "2023-01-01T00:00:00.000Z" },
    },
    {
      text: "mappe",
      locale: "nb",
      status: "preferred",
      validity: {
        validFrom: "2024-01-01T00:00:00.000Z",
        tags: { market: "nordics" },
      },
    },
  ],
};

const input: EvolutionInput = { concept };

describe("EvolutionRoadmap", () => {
  it("mounts and shows a locale label", () => {
    const model = buildEvolutionModel(input, { now: NOW });
    render(<EvolutionRoadmap model={model} />);

    // The lane labels render the locale (uppercased in the label atom).
    expect(screen.getByText("en")).toBeDefined();
    // The "now" tag anchors the right edge of the roadmap.
    expect(screen.getByText("now")).toBeDefined();
  });
});

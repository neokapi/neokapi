import { describe, expect, it } from "vitest";
import { PROJECT_SAMPLES } from "./samples";

// The playground seeds a sample project into the terminal's working directory
// and stages the funnel with no -p, so every step finds the project by walking
// up to a `kapi.yaml`. A recipe under any other name leaves the reader with
// "no kapi project found" on the first command.
describe("PROJECT_SAMPLES", () => {
  it.each(PROJECT_SAMPLES.map((s) => [s.id, s] as const))(
    "%s seeds its recipe as kapi.yaml",
    (_id, sample) => {
      expect(sample.recipeName).toBe("kapi.yaml");
      const recipe = sample.files.find((f) => f.path === sample.recipeName);
      expect(recipe?.content).toMatch(/^version: v1\n/);
    },
  );
});

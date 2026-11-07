import { render, screen } from "./testUtils";
import { describe, it, expect } from "vite-plus/test";
import {
  LabelledCoordinates,
  coordinateDifferences,
  displayCoordinateValue,
  sharedCoordinates,
} from "../components/PointCoordinates";

describe("point coordinates", () => {
  it("finds the coordinates every point holds with the same value", () => {
    expect(
      sharedCoordinates([
        { age: "13-", brand: "kapimart", channel: "web" },
        { age: "13-", brand: "kapimart", channel: "email" },
        undefined,
      ]),
    ).toEqual({ age: "13-", brand: "kapimart" });
    expect(sharedCoordinates([])).toEqual({});
  });

  it("keeps only the coordinates that differ from the shared set", () => {
    expect(
      coordinateDifferences(
        { age: "13-", brand: "kapimart", channel: "web" },
        { age: "13-", brand: "kapimart" },
      ),
    ).toEqual([["channel", "web"]]);
    expect(coordinateDifferences(undefined, { brand: "kapimart" })).toEqual([]);
  });

  it("draws an open numeric range with an en dash and leaves other values alone", () => {
    expect(displayCoordinateValue("13-")).toBe("13–");
    expect(displayCoordinateValue("13-17")).toBe("13–17");
    expect(displayCoordinateValue("-12")).toBe("–12");
    expect(displayCoordinateValue("pt-BR")).toBe("pt-BR");
    expect(displayCoordinateValue("-")).toBe("-");
  });

  it("labels each chip with its axis", () => {
    render(
      <LabelledCoordinates
        entries={[
          ["age", "13-"],
          ["brand", "kapimart"],
        ]}
        testId="coords"
      />,
    );
    const coords = screen.getByTestId("coords");
    expect(coords).toHaveTextContent("age");
    expect(coords).toHaveTextContent("13–");
    expect(coords).toHaveTextContent("brand");
  });
});

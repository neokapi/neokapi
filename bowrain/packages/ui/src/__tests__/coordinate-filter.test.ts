import { describe, it, expect } from "vite-plus/test";
import {
  formatCoordinateFilter,
  parseCoordinateFilter,
} from "../context-hub/profiles/coordinateFilter";

describe("parseCoordinateFilter", () => {
  it("reads one axis", () => {
    expect(parseCoordinateFilter("brand=acme")).toEqual({ coordinates: { brand: "acme" } });
  });

  it("reads several axes and trims whitespace", () => {
    expect(parseCoordinateFilter(" brand = acme , channel = support ")).toEqual({
      coordinates: { brand: "acme", channel: "support" },
    });
  });

  it("treats empty text as the whole space", () => {
    expect(parseCoordinateFilter("")).toEqual({});
    expect(parseCoordinateFilter("   ")).toEqual({});
  });

  it("refuses a half-written axis rather than dropping it", () => {
    // Dropping it would widen the grant, since an axis a filter does not name is
    // an axis it does not constrain.
    expect(parseCoordinateFilter("brand").error).toBeTruthy();
    expect(parseCoordinateFilter("brand=").error).toBeTruthy();
    expect(parseCoordinateFilter("=acme").error).toBeTruthy();
    expect(parseCoordinateFilter("brand=acme,channel").error).toBeTruthy();
  });

  it("refuses one axis given two values", () => {
    expect(parseCoordinateFilter("brand=acme,brand=other").error).toBeTruthy();
    expect(parseCoordinateFilter("brand=acme,brand=acme")).toEqual({
      coordinates: { brand: "acme" },
    });
  });
});

describe("formatCoordinateFilter", () => {
  it("sorts axes so the text is stable", () => {
    expect(formatCoordinateFilter({ product: "docs", brand: "acme" })).toBe(
      "brand=acme,product=docs",
    );
  });

  it("renders the whole space as empty text", () => {
    expect(formatCoordinateFilter(undefined)).toBe("");
    expect(formatCoordinateFilter({})).toBe("");
  });

  it("round-trips", () => {
    const coordinates = { brand: "acme", channel: "support" };
    expect(parseCoordinateFilter(formatCoordinateFilter(coordinates))).toEqual({ coordinates });
  });
});

import { describe, expect, it, beforeEach } from "vitest";
import {
  searchPlan,
  searchSeats,
  isIntendedPlan,
  stashIntendedPlan,
  readIntendedPlan,
  clearIntendedPlan,
} from "./intended-plan";

function wipeCookies() {
  for (const c of document.cookie.split("; ")) {
    const name = c.split("=")[0];
    if (name) document.cookie = `${name}=; path=/; max-age=0`;
  }
}

describe("searchPlan", () => {
  it("keeps the known self-serve plans", () => {
    expect(searchPlan("pro")).toBe("pro");
    expect(searchPlan("team")).toBe("team");
  });

  it("drops non-self-serve and junk (the server re-validates anyway)", () => {
    expect(searchPlan("free")).toBeUndefined();
    expect(searchPlan("enterprise")).toBeUndefined();
    expect(searchPlan("deluxe")).toBeUndefined();
    expect(searchPlan(42)).toBeUndefined();
    expect(searchPlan(undefined)).toBeUndefined();
    expect(searchPlan(null)).toBeUndefined();
  });
});

describe("searchSeats", () => {
  it("accepts the JSON-parsed number and a hand-typed string", () => {
    expect(searchSeats(3)).toBe(3);
    expect(searchSeats("5")).toBe(5);
  });

  it("rejects non-positive, non-integer, and junk", () => {
    expect(searchSeats(0)).toBeUndefined();
    expect(searchSeats(-2)).toBeUndefined();
    expect(searchSeats(2.5)).toBeUndefined();
    expect(searchSeats("abc")).toBeUndefined();
    expect(searchSeats(undefined)).toBeUndefined();
  });
});

describe("isIntendedPlan", () => {
  it("is a type guard for the self-serve ids", () => {
    expect(isIntendedPlan("pro")).toBe(true);
    expect(isIntendedPlan("team")).toBe(true);
    expect(isIntendedPlan("free")).toBe(false);
    expect(isIntendedPlan(123)).toBe(false);
  });
});

describe("intended-plan cookie survives the auth round-trip", () => {
  beforeEach(wipeCookies);

  it("round-trips the plan (models the OIDC redirect: stash before login, read after return)", () => {
    stashIntendedPlan({ plan: "pro" });
    // Simulates the app coming back up after the identity-provider redirect,
    // where the query string is gone but the cookie remains.
    expect(readIntendedPlan()).toEqual({ plan: "pro", seats: undefined });
  });

  it("carries a Team seat count through", () => {
    stashIntendedPlan({ plan: "team", seats: 3 });
    expect(readIntendedPlan()).toEqual({ plan: "team", seats: 3 });
  });

  it("returns null when nothing is stashed", () => {
    expect(readIntendedPlan()).toBeNull();
  });

  it("clears the stash once consumed", () => {
    stashIntendedPlan({ plan: "pro" });
    clearIntendedPlan();
    expect(readIntendedPlan()).toBeNull();
  });

  it("ignores a tampered cookie carrying a non-self-serve plan", () => {
    document.cookie = `bowrain_intended_plan=${encodeURIComponent("plan=enterprise")}; path=/`;
    expect(readIntendedPlan()).toBeNull();
  });
});

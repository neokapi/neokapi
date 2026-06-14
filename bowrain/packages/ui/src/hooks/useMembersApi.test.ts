import { describe, it, expect } from "vite-plus/test";
import type { Membership, User } from "../types/api";
import { buildNameResolver } from "./useMembersApi";

function user(over: Partial<User> = {}): User {
  return {
    id: "u-0",
    email: "person@example.com",
    name: "A Person",
    avatar_url: "",
    ...over,
  };
}

function member(userId: string, over: Partial<User> = {}): Membership {
  return {
    user_id: userId,
    workspace_id: "ws-1",
    role: "member",
    user: user({ id: userId, ...over }),
  };
}

describe("buildNameResolver", () => {
  it("resolves a known user_id to its member's name", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "Ada Lovelace" })]);
    expect(nameOf("u-1")).toBe("Ada Lovelace");
  });

  it("falls back to email when the name is empty or whitespace", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "   ", email: "ada@example.com" })]);
    expect(nameOf("u-1")).toBe("ada@example.com");
  });

  it("falls back to the raw id when both name and email are empty", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "", email: "" })]);
    expect(nameOf("u-1")).toBe("u-1");
  });

  it("returns the raw id for an unknown user (former member, system actor)", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "Ada" })]);
    expect(nameOf("00000000-unknown")).toBe("00000000-unknown");
  });

  it("returns an empty string for empty, null, or undefined ids", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "Ada" })]);
    expect(nameOf("")).toBe("");
    expect(nameOf(null)).toBe("");
    expect(nameOf(undefined)).toBe("");
  });

  it("handles no members (loading / empty workspace) without throwing", () => {
    expect(buildNameResolver(undefined)("u-1")).toBe("u-1");
    expect(buildNameResolver([])("u-1")).toBe("u-1");
  });

  it("trims surrounding whitespace from the resolved name", () => {
    const nameOf = buildNameResolver([member("u-1", { name: "  Ada Lovelace  " })]);
    expect(nameOf("u-1")).toBe("Ada Lovelace");
  });
});

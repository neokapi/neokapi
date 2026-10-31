// @vitest-environment jsdom
import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";

import { NoticedBy, memberName, toolName } from "../NoticedBy";

describe("toolName", () => {
  it("names kapi itself once", () => {
    expect(toolName("kapi")).toBe("kapi");
    expect(toolName(undefined)).toBe("kapi");
    expect(toolName("")).toBe("kapi");
  });

  it("names another tool after kapi", () => {
    expect(toolName("check")).toBe("kapi check");
  });
});

describe("NoticedBy", () => {
  it("shows a suggestion kapi filed as noticed by kapi", () => {
    render(<NoticedBy who={{ kind: "tool", name: "kapi" }} />);
    expect(screen.getByText("kapi")).toBeTruthy();
    expect(screen.queryByText("kapi kapi")).toBeNull();
  });
});

describe("memberName", () => {
  const nameOf = (id: string) => (id === "u-mira" ? "Mira Holt" : id);

  it("names a person by the member list's name for their id", () => {
    expect(memberName({ kind: "person", name: "u-mira" }, nameOf)).toBe("Mira Holt");
  });

  it("names a person with no name as a member, never as the reader", () => {
    expect(memberName({ kind: "person" }, nameOf)).toBe("A member");
    render(<NoticedBy who={{ kind: "person" }} nameOf={(who) => memberName(who, nameOf)} />);
    expect(screen.getByText("A member")).toBeInTheDocument();
  });

  it("leaves an assistant and a tool to NoticedBy", () => {
    expect(memberName({ kind: "agent", name: "claude" }, nameOf)).toBeUndefined();
    expect(memberName({ kind: "tool" }, nameOf)).toBeUndefined();
  });
});

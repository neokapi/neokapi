import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "./testUtils";

import { NoticedBy, toolName } from "../components/ContextDigestDetail";

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

import { describe, it, expect } from "vitest";
import { parseAppError } from "../lib/parse-app-error";

describe("parseAppError", () => {
  // The exact string a user reported seeing rendered raw in the UI.
  const reported =
    '{"message":"no default flow configured — add a flow or pick one to run","cause":{},"kind":"RuntimeError"}';

  const cases: Array<{
    name: string;
    input: unknown;
    title: string;
    detail?: string;
    hasRaw: boolean;
  }> = [
    {
      name: "the exact reported Wails envelope string",
      input: reported,
      title: "no default flow configured — add a flow or pick one to run",
      hasRaw: true,
    },
    {
      name: "Wails envelope object",
      input: { message: "file not found", kind: "IOError", cause: {} },
      title: "file not found",
      hasRaw: true,
    },
    {
      name: "envelope with a string cause becomes the detail",
      input: { message: "push failed", cause: "connection refused", kind: "NetError" },
      title: "push failed",
      detail: "connection refused",
      hasRaw: true,
    },
    {
      name: "envelope with an object cause carrying a message",
      input: { message: "flow failed", cause: { message: "tool exited 1" } },
      title: "flow failed",
      detail: "tool exited 1",
      hasRaw: true,
    },
    {
      name: "nested JSON string message (one parse attempt)",
      input: JSON.stringify({
        message: '{"message":"inner friendly line","kind":"RuntimeError"}',
        kind: "WrapError",
      }),
      title: "inner friendly line",
      hasRaw: true,
    },
    {
      name: "plain string passes through",
      input: "something broke",
      title: "something broke",
      hasRaw: false,
    },
    {
      name: "string that merely resembles JSON but is invalid stays verbatim",
      input: "{not json}",
      title: "{not json}",
      hasRaw: false,
    },
    {
      name: "envelope using error field",
      input: '{"error":"permission denied"}',
      title: "permission denied",
      hasRaw: true,
    },
    {
      name: "empty string falls back",
      input: "",
      title: "Something went wrong",
      hasRaw: false,
    },
    {
      name: "null falls back",
      input: null,
      title: "Something went wrong",
      hasRaw: false,
    },
    {
      name: "undefined falls back",
      input: undefined,
      title: "Something went wrong",
      hasRaw: false,
    },
    {
      name: "object without a message keeps raw payload",
      input: { code: 500 },
      title: "Something went wrong",
      hasRaw: true,
    },
    {
      name: "JSON array string keeps raw payload",
      input: "[1,2,3]",
      title: "Something went wrong",
      hasRaw: true,
    },
    {
      name: "number coerces to string",
      input: 42,
      title: "42",
      hasRaw: false,
    },
  ];

  it.each(cases)("$name", ({ input, title, detail, hasRaw }) => {
    const parsed = parseAppError(input);
    expect(parsed.title).toBe(title);
    if (detail !== undefined) expect(parsed.detail).toBe(detail);
    if (hasRaw) {
      expect(parsed.raw).toBeDefined();
    } else {
      expect(parsed.raw).toBeUndefined();
    }
  });

  it("keeps the full structured payload in raw for the reported envelope", () => {
    const parsed = parseAppError(reported);
    expect(parsed.raw).toContain('"kind": "RuntimeError"');
    expect(parsed.raw).toContain('"message"');
  });

  it("uses an Error's message as the title and keeps the stack as raw", () => {
    const err = new Error("boom");
    const parsed = parseAppError(err);
    expect(parsed.title).toBe("boom");
    expect(parsed.raw).toBe(err.stack);
  });

  it("parses an Error whose message is a JSON envelope", () => {
    const err = new Error('{"message":"friendly from envelope","kind":"RuntimeError"}');
    const parsed = parseAppError(err);
    expect(parsed.title).toBe("friendly from envelope");
    expect(parsed.raw).toContain("RuntimeError");
  });

  it("falls back to the Error name when the message is empty", () => {
    const err = new Error("");
    err.name = "TypeError";
    const parsed = parseAppError(err);
    expect(parsed.title).toBe("TypeError");
  });
});

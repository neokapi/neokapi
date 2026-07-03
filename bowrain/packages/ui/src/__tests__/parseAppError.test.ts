import { describe, it, expect } from "vite-plus/test";
import { parseAppError, hasStructuredRaw } from "../errors/parseAppError";

describe("parseAppError", () => {
  describe("envelope shapes", () => {
    const cases: Array<{
      name: string;
      input: unknown;
      title: string;
      detail?: string;
      code?: string;
      status?: number;
      hasRaw: boolean;
    }> = [
      {
        name: "REST envelope {error}",
        input: { error: "project not found" },
        title: "Project not found",
        hasRaw: true,
      },
      {
        name: "REST envelope {error, details}",
        input: { error: "invalid request body", details: "field name is required" },
        title: "Invalid request body",
        detail: "field name is required",
        hasRaw: true,
      },
      {
        name: "REST envelope {error, code} maps known codes to friendly phrasing",
        input: { error: "forbidden", code: "forbidden" },
        title: "You don't have permission to do that",
        code: "forbidden",
        hasRaw: true,
      },
      {
        name: "client envelope {message, kind, cause}",
        input: { message: "push failed", kind: "sync", cause: "connection reset" },
        title: "Push failed",
        detail: "connection reset",
        hasRaw: true,
      },
      {
        name: "plain string",
        input: "Failed to save changes",
        title: "Failed to save changes",
        hasRaw: false,
      },
      {
        name: "plain string containing a JSON envelope gets one parse attempt",
        input: '{"error":"stream is required"}',
        title: "Stream is required",
        hasRaw: true,
      },
      {
        name: "Error instance",
        input: new Error("network unreachable"),
        title: "Network unreachable",
        hasRaw: false,
      },
      {
        name: "adapter Error '<status>: <json body>'",
        input: new Error('404: {"error":"block not found"}'),
        title: "Block not found",
        status: 404,
        hasRaw: true,
      },
      {
        name: "adapter Error with {error, code} body",
        input: new Error('409: {"error":"slug is already in use","code":"conflict"}'),
        title: "That change conflicts with the current state",
        detail: "Slug is already in use",
        code: "conflict",
        status: 409,
        hasRaw: true,
      },
      {
        name: "adapter Error with HTML body keeps only the status phrasing",
        input: new Error("502: <html><body>Bad gateway</body></html>"),
        title: "The server is temporarily unreachable",
        status: 502,
        hasRaw: false,
      },
      {
        name: "adapter Error with plain-text body",
        input: new Error("500: database is locked"),
        title: "The server hit an unexpected error",
        detail: "Database is locked",
        status: 500,
        hasRaw: false,
      },
      {
        name: "nested JSON inside a {message} envelope gets one parse attempt",
        input: { message: '{"error":"locale is required"}' },
        title: "Locale is required",
        hasRaw: true,
      },
      {
        name: "known server message maps to friendly phrasing",
        input: { error: "not a member of this workspace" },
        title: "You're not a member of this workspace",
        hasRaw: true,
      },
      {
        name: "rate limit prefix maps to friendly phrasing",
        input: { error: "rate limit exceeded: max 10 pushes per minute per project" },
        title: "Too many requests",
        detail: "Rate limit exceeded: max 10 pushes per minute per project",
        hasRaw: true,
      },
    ];

    for (const c of cases) {
      it(c.name, () => {
        const parsed = parseAppError(c.input);
        expect(parsed.title).toBe(c.title);
        if (c.detail !== undefined) expect(parsed.detail).toBe(c.detail);
        if (c.code !== undefined) expect(parsed.code).toBe(c.code);
        if (c.status !== undefined) expect(parsed.status).toBe(c.status);
        expect(hasStructuredRaw(parsed)).toBe(c.hasRaw);
      });
    }
  });

  it("uses the fallback title for null/undefined/empty", () => {
    expect(parseAppError(null, "Couldn't load blocks").title).toBe("Couldn't load blocks");
    expect(parseAppError(undefined).title).toBe("Something went wrong");
    expect(parseAppError("").title).toBe("Something went wrong");
  });

  it("preserves the raw payload verbatim", () => {
    const envelope = { error: "flow not found", code: "not_found" };
    expect(parseAppError(envelope).raw).toBe(envelope);
  });

  it("preserves the parsed body as raw for adapter errors", () => {
    const parsed = parseAppError(new Error('404: {"error":"flow not found"}'));
    expect(parsed.raw).toEqual({ error: "flow not found" });
  });

  it("attaches hints for known codes and statuses", () => {
    expect(parseAppError({ error: "nope", code: "rate_limited" }).hint).toMatch(/wait/i);
    expect(parseAppError(new Error("503: ")).hint).toMatch(/try again/i);
  });

  it("truncates very long messages", () => {
    const long = "x".repeat(500);
    const parsed = parseAppError(long);
    expect(parsed.title.length).toBeLessThanOrEqual(160);
    expect(parsed.title.endsWith("…")).toBe(true);
  });

  it("picks up status/code fields attached to Error instances", () => {
    const err = new Error("boom") as Error & { status: number; code: string };
    err.status = 403;
    err.code = "forbidden";
    const parsed = parseAppError(err);
    expect(parsed.status).toBe(403);
    expect(parsed.code).toBe("forbidden");
    expect(parsed.hint).toMatch(/admin/i);
  });

  it("handles arrays and non-object values without throwing", () => {
    expect(parseAppError([1, 2, 3]).title).toBe("Something went wrong");
    expect(hasStructuredRaw(parseAppError([1, 2, 3]))).toBe(true);
    expect(parseAppError(42).title).toBe("Something went wrong");
    expect(parseAppError(true).title).toBe("Something went wrong");
  });

  it("does not double-parse nested JSON (single parse attempt only)", () => {
    // The inner message is itself a JSON string; after one parse we must not
    // keep unwrapping.
    const doubly = JSON.stringify({ error: JSON.stringify({ error: "inner" }) });
    const parsed = parseAppError(doubly);
    expect(parsed.title).toBe('{"error":"inner"}');
  });
});

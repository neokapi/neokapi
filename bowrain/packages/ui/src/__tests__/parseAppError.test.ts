import { describe, it, expect } from "vite-plus/test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parseAppError, hasStructuredRaw, PERMISSION_REFUSALS } from "../errors/parseAppError";

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

  describe("reference (correlation ID)", () => {
    it("extracts reference from the REST envelope body", () => {
      const parsed = parseAppError({ error: "internal server error", reference: "req-abc-123" });
      expect(parsed.reference).toBe("req-abc-123");
    });

    it("extracts reference from a stringified '<status>: <json>' body", () => {
      const parsed = parseAppError(
        new Error(
          `500: ${JSON.stringify({ error: "internal server error", reference: "req-xyz" })}`,
        ),
      );
      expect(parsed.status).toBe(500);
      expect(parsed.reference).toBe("req-xyz");
    });

    it("picks up a reference attached to an Error instance (header-derived)", () => {
      const err = Object.assign(new Error("502: <html>bad gateway</html>"), {
        reference: "req-from-header",
      });
      const parsed = parseAppError(err);
      expect(parsed.reference).toBe("req-from-header");
    });

    it("leaves reference undefined when absent", () => {
      expect(parseAppError({ error: "nope" }).reference).toBeUndefined();
    });
  });

  it("does not double-parse nested JSON (single parse attempt only)", () => {
    // The inner message is itself a JSON string; after one parse we must not
    // keep unwrapping.
    const doubly = JSON.stringify({ error: JSON.stringify({ error: "inner" }) });
    const parsed = parseAppError(doubly);
    expect(parsed.title).toBe('{"error":"inner"}');
  });

  describe("per-status remedies", () => {
    // The remedy a status maps to presumes a cause: "ask an admin" presumes a
    // missing grant. A response that names its own cause renders that sentence
    // and nothing more; the status phrase stands in only when the body carries
    // no explanation at all.
    const refusal = "separation of duties: you cannot review or approve your own work";

    it("renders a 403 that states its reason without the permission remedy", () => {
      const parsed = parseAppError(new Error(`403: ${JSON.stringify({ error: refusal })}`));
      expect(parsed.title).toBe("Separation of duties: you cannot review or approve your own work");
      expect(parsed.detail).toBeUndefined();
      expect(parsed.hint).toBeUndefined();
      expect(parsed.status).toBe(403);
    });

    it("renders a 409 that states its reason without the refresh remedy", () => {
      const parsed = parseAppError(
        new Error(
          '409: {"error":"governed change requires a change-set","detail":"deleting concepts","hint":"open a change-set"}',
        ),
      );
      expect(parsed.title).toBe("Governed change requires a change-set");
      expect(parsed.hint).toBeUndefined();
    });

    it("renders a {message} envelope carrying a status without a status remedy", () => {
      const parsed = parseAppError(new Error('403: {"message":"this file is read-only"}'));
      expect(parsed.title).toBe("This file is read-only");
      expect(parsed.hint).toBeUndefined();
      expect(parsed.status).toBe(403);
    });

    it("renders a 5xx envelope as the server's sentence and reference alone", () => {
      const parsed = parseAppError(
        new Error(
          `500: ${JSON.stringify({
            error: "internal server error",
            message:
              "The server encountered an unexpected error while handling this request. Quote the reference when reporting it.",
            reference: "req-5",
          })}`,
        ),
      );
      expect(parsed.title).toMatch(/^The server encountered an unexpected error/);
      expect(parsed.hint).toBeUndefined();
      expect(parsed.reference).toBe("req-5");
    });

    it("keeps the permission remedy for the refusals the server writes for a missing grant", () => {
      for (const body of [
        '{"error":"insufficient project permissions"}',
        '{"error":"insufficient permissions"}',
        '{"error":"no review permission for fr"}',
        '{"error":"no access to language: fr"}',
        '{"error":"demoting a signed-off target requires review permission"}',
      ]) {
        const parsed = parseAppError(new Error(`403: ${body}`));
        expect(parsed.title, body).toBe("You don't have permission to do that");
        expect(parsed.hint, body).toBe("Ask a workspace admin to grant you access.");
        expect(parsed.detail, body).toBeDefined();
      }
    });

    it("falls back to the status phrasing for a bare 403", () => {
      const parsed = parseAppError(new Error("403: "));
      expect(parsed.title).toBe("You don't have permission to do that");
      expect(parsed.detail).toBeUndefined();
      expect(parsed.hint).toBe("Ask a workspace admin to grant you access.");
    });

    it("falls back to the status phrasing for a 403 whose body is not JSON", () => {
      const html = parseAppError(new Error("403: <html><body>Forbidden</body></html>"));
      expect(html.title).toBe("You don't have permission to do that");
      expect(html.detail).toBeUndefined();
      expect(html.hint).toBe("Ask a workspace admin to grant you access.");

      const text = parseAppError(new Error("403: access denied by the gateway"));
      expect(text.title).toBe("You don't have permission to do that");
      expect(text.detail).toBe("Access denied by the gateway");
      expect(text.hint).toBe("Ask a workspace admin to grant you access.");
    });

    it("applies a status attached to an Error only when its message said nothing", () => {
      const silent = parseAppError(Object.assign(new Error(""), { status: 403 }));
      expect(silent.title).toBe("You don't have permission to do that");
      expect(silent.hint).toBe("Ask a workspace admin to grant you access.");

      const spoken = parseAppError(Object.assign(new Error(refusal), { status: 403 }));
      expect(spoken.title).toBe("Separation of duties: you cannot review or approve your own work");
      expect(spoken.hint).toBeUndefined();
      expect(spoken.status).toBe(403);
    });
  });
});

describe("the permission refusals are the ones the server writes", () => {
  // The UI attaches the "ask an admin" remedy to a fixed list of server
  // sentences. A sentence reworded on the Go side would silently lose its
  // remedy, and a sentence listed here that no handler writes is dead copy, so
  // both files are read and held against each other.
  const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..", "..");
  const server = [
    "middleware_auth.go",
    "review_governance.go",
    "handlers_editor_bulk.go",
    "handlers_governance.go",
  ]
    .map((file) => readFileSync(join(REPO_ROOT, "bowrain/server", file), "utf8"))
    .join("\n");

  it("lists only refusals a server handler writes", () => {
    expect(PERMISSION_REFUSALS.length).toBeGreaterThan(0);
    for (const refusal of PERMISSION_REFUSALS) {
      expect(server, refusal).toContain(`"${refusal}`);
    }
  });

  it("renders the separation-of-duties sentence the server writes without a remedy", () => {
    const declared = /const sodRefusal = "([^"]+)"/.exec(server);
    expect(declared).not.toBeNull();
    const parsed = parseAppError(new Error(`403: ${JSON.stringify({ error: declared![1] })}`));
    expect(parsed.title).toBe("Separation of duties: you cannot review or approve your own work");
    expect(parsed.hint).toBeUndefined();
  });
});

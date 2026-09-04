import { describe, it, expect } from "vite-plus/test";
import { ApiError, apiErrorFromResponse } from "../errors/ApiError";
import { parseAppError } from "../errors/parseAppError";

/**
 * The typed ApiError the REST adapter throws, and how parseAppError turns the
 * standard server envelope `{error, message?, reference?, ...details}` into
 * friendly copy. Covers the full envelope, every legacy shape, and the
 * central code → copy map.
 */

const limitBody = JSON.stringify({
  current: 1,
  error: "project_limit_reached",
  limit: 1,
  message: "This workspace's plan allows 1 project(s) and it already has 1.",
  reference: "req-abc-123",
});

describe("apiErrorFromResponse", () => {
  it("parses the full envelope into structured fields", () => {
    const err = apiErrorFromResponse(403, limitBody, "hdr-ref");
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(403);
    expect(err.code).toBe("project_limit_reached");
    // The body's reference wins over the header.
    expect(err.referenceId).toBe("req-abc-123");
    expect(err.details).toEqual({ current: 1, limit: 1 });
    // `.message` is the server's human sentence, so even raw `.message`
    // renderings never show JSON.
    expect(err.message).toBe("This workspace's plan allows 1 project(s) and it already has 1.");
  });

  it("tolerates a legacy envelope without message/reference", () => {
    const err = apiErrorFromResponse(
      403,
      '{"current":1,"error":"project_limit_reached","limit":1}',
    );
    expect(err.code).toBe("project_limit_reached");
    expect(err.referenceId).toBeUndefined();
    expect(err.details).toEqual({ current: 1, limit: 1 });
    expect(err.message).toBe("project_limit_reached");
  });

  it("tolerates the reference_id spelling", () => {
    const err = apiErrorFromResponse(400, '{"error":"invalid_grant","reference_id":"req-9"}');
    expect(err.referenceId).toBe("req-9");
  });

  it("falls back to the X-Request-ID header and the legacy message for non-JSON bodies", () => {
    const err = apiErrorFromResponse(502, "<html>bad gateway</html>", "req-hdr-7");
    expect(err.code).toBeUndefined();
    expect(err.referenceId).toBe("req-hdr-7");
    expect(err.message).toBe("502: <html>bad gateway</html>");
  });
});

describe("parseAppError(ApiError) — the central code → copy map", () => {
  it("project_limit_reached reads as actionable copy with the reference", () => {
    const parsed = parseAppError(apiErrorFromResponse(403, limitBody, null));
    expect(parsed.title).toBe("This workspace's plan allows 1 project and already has 1");
    expect(parsed.hint).toBe("Create a new workspace, choose another, or get in touch.");
    expect(parsed.reference).toBe("req-abc-123");
    expect(parsed.status).toBe(403);
  });

  it("legacy project_limit_reached (no message/reference) still maps, pluralized", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(403, '{"current":5,"error":"project_limit_reached","limit":3}'),
    );
    expect(parsed.title).toBe("This workspace's plan allows 3 projects and already has 5");
    expect(parsed.hint).toBe("Create a new workspace, choose another, or get in touch.");
    expect(parsed.reference).toBeUndefined();
  });

  it("upgrade_required names the minimum plan", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(
        403,
        '{"error":"upgrade_required","feature":"connectors-git","minimum_plan":"pro"}',
      ),
    );
    expect(parsed.title).toBe("This feature is not included in the workspace's current plan");
    expect(parsed.hint).toBe("Upgrade to the pro plan to enable it.");
  });

  it("credits_exhausted and insufficient_credits share the credits copy", () => {
    for (const code of ["credits_exhausted", "insufficient_credits"]) {
      const parsed = parseAppError(apiErrorFromResponse(429, JSON.stringify({ error: code })));
      expect(parsed.title).toBe("This workspace has used its weekly credits");
      expect(parsed.hint).toContain("Upgrade the plan");
    }
  });

  it("elevation_required asks for a security check", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(409, '{"error":"elevation_required","elevate_url":"/elevate"}'),
    );
    expect(parsed.title).toBe("This action needs a recent security check");
  });

  it("a taken handle reads as friendly copy", () => {
    const parsed = parseAppError(apiErrorFromResponse(409, '{"error":"slug is already in use"}'));
    expect(parsed.title).toBe("That handle is already taken");
    expect(parsed.hint).toBe("Choose a different handle.");
  });

  it("unmapped codes fall back to the server's message sentence", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(
        403,
        '{"error":"signups are currently closed","message":"New signups are currently closed on this server.","reference":"req-2"}',
      ),
    );
    expect(parsed.title).toBe("New signups are currently closed on this server.");
    expect(parsed.reference).toBe("req-2");
  });

  it("keeps the header-derived reference for non-envelope bodies", () => {
    const parsed = parseAppError(apiErrorFromResponse(502, "", "req-hdr-8"));
    expect(parsed.reference).toBe("req-hdr-8");
    expect(parsed.status).toBe(502);
  });
});

describe("parseAppError(ApiError) — a 403 that states its reason", () => {
  // The review surface's banner: the server refuses the reviewer's own work
  // with one sentence, and no grant fixes it, so no remedy is appended. The
  // per-status remedy is reserved for a 403 that carries no explanation.
  const refusal = "separation of duties: you cannot review or approve your own work";

  it("renders a separation-of-duties refusal as its own sentence", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(403, JSON.stringify({ error: refusal }), "req-sod"),
    );
    expect(parsed.title).toBe("Separation of duties: you cannot review or approve your own work");
    expect(parsed.detail).toBeUndefined();
    expect(parsed.hint).toBeUndefined();
    expect(parsed.status).toBe(403);
    expect(parsed.reference).toBe("req-sod");
  });

  it("keeps the permission remedy for a refusal written for a missing grant", () => {
    const parsed = parseAppError(
      apiErrorFromResponse(403, '{"error":"insufficient project permissions"}'),
    );
    expect(parsed.title).toBe("You don't have permission to do that");
    expect(parsed.detail).toBe("Insufficient project permissions");
    expect(parsed.hint).toBe("Ask a workspace admin to grant you access.");
  });

  it("falls back to the status phrasing for a 403 with no body", () => {
    const parsed = parseAppError(apiErrorFromResponse(403, "", "req-hdr-3"));
    expect(parsed.title).toBe("You don't have permission to do that");
    expect(parsed.hint).toBe("Ask a workspace admin to grant you access.");
    expect(parsed.reference).toBe("req-hdr-3");
  });

  it("falls back to the status phrasing for a 403 whose body is not JSON", () => {
    const html = parseAppError(apiErrorFromResponse(403, "<html><body>Forbidden</body></html>"));
    expect(html.title).toBe("You don't have permission to do that");
    expect(html.detail).toBeUndefined();
    expect(html.hint).toBe("Ask a workspace admin to grant you access.");

    const text = parseAppError(apiErrorFromResponse(403, "Forbidden"));
    expect(text.title).toBe("You don't have permission to do that");
    expect(text.detail).toBe("Forbidden");
    expect(text.hint).toBe("Ask a workspace admin to grant you access.");
  });
});

describe("parseAppError on raw envelope objects (no ApiError wrapper)", () => {
  it("reads message/reference from the envelope", () => {
    const parsed = parseAppError({
      error: "some_new_code",
      message: "A plain factual sentence from the server.",
      reference: "req-77",
    });
    expect(parsed.title).toBe("A plain factual sentence from the server.");
    expect(parsed.reference).toBe("req-77");
  });

  it("tolerates reference_id on the envelope", () => {
    const parsed = parseAppError({ error: "x failed", reference_id: "req-88" });
    expect(parsed.reference).toBe("req-88");
  });
});

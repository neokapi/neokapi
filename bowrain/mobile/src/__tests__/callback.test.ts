import { readCallback } from "../lib/callback";

describe("readCallback", () => {
  const state = "6f1c2c43-1d0e-4c55-9a7b-1f7d8c3e2a10";

  it("returns the one-time code when the state matches", () => {
    expect(readCallback(`bowrain://auth/callback?code=abc&state=${state}`, state)).toEqual({
      ok: true,
      code: "abc",
    });
  });

  it("refuses a different or missing state before reading anything else", () => {
    expect(readCallback("bowrain://auth/callback?code=abc&state=other", state)).toEqual({
      ok: false,
      reason: "state_mismatch",
    });
    expect(readCallback("bowrain://auth/callback?code=abc", state)).toEqual({
      ok: false,
      reason: "state_mismatch",
    });
    expect(readCallback(`bowrain://auth/callback?code=abc&state=${state}`, "")).toMatchObject({
      ok: false,
    });
  });

  it("never takes a token from the URL", () => {
    const url = `bowrain://auth/callback?token=eyJhbGciOi&refresh_token=rt&state=${state}`;
    const result = readCallback(url, state);
    expect(result).toEqual({ ok: false, reason: "missing_code" });
    expect(JSON.stringify(result)).not.toContain("eyJhbGciOi");
  });

  it("reports an identity provider error", () => {
    expect(
      readCallback(
        `bowrain://auth/callback?error=access_denied&error_description=No&state=${state}`,
        state,
      ),
    ).toEqual({ ok: false, reason: "error", message: "No" });
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { checkBrand, lookupTerms, translate } from "../api";

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "",
    json: async () => body,
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("addin api client", () => {
  it("posts a brand check and parses findings", async () => {
    const fetchMock = mockFetch(200, {
      profile: "Test",
      score: 80,
      findings: [{ category: "vocabulary", severity: "minor", message: "avoid 'utilize'" }],
    });
    vi.stubGlobal("fetch", fetchMock);

    const res = await checkBrand("Please utilize this.", "tok");
    expect(res.score).toBe(80);
    expect(res.findings).toHaveLength(1);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/addin/check");
    expect((init as RequestInit).method).toBe("POST");
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok");
    expect(JSON.parse((init as RequestInit).body as string)).toMatchObject({
      text: "Please utilize this.",
    });
  });

  it("posts a terms lookup", async () => {
    const fetchMock = mockFetch(200, {
      profile: "Test",
      matches: [{ term: "utilize", status: "forbidden" }],
    });
    vi.stubGlobal("fetch", fetchMock);

    const res = await lookupTerms("utilize");
    expect(res.matches[0].term).toBe("utilize");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/addin/terms");
  });

  it("posts a translate request with the target locale", async () => {
    const fetchMock = mockFetch(200, {
      translation: "Bonjour",
      source_locale: "en",
      target_locale: "fr",
      provider: "demo",
    });
    vi.stubGlobal("fetch", fetchMock);

    const res = await translate("Hello", "fr");
    expect(res.translation).toBe("Bonjour");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({
      text: "Hello",
      target_locale: "fr",
    });
  });

  it("throws with the server error detail on failure", async () => {
    const fetchMock = mockFetch(400, { error: "target_locale is required" });
    vi.stubGlobal("fetch", fetchMock);

    await expect(translate("Hello", "")).rejects.toThrow(/target_locale is required/);
  });
});

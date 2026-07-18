import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * These tests pin the billing adapter against the payloads the SERVER actually
 * sends (Go, snake_case).
 *
 * They exist because the billing types were hand-written in camelCase against an
 * API that never existed, and nothing caught it: `overview.credits` was always
 * undefined, so the credit card silently never rendered; the usage breakdown
 * rendered the string "undefined"; the ledger called a route that does not exist;
 * and checkout redirected to `undefined`. A mapping that is asserted against the
 * real wire shape is the only thing that keeps that from happening again.
 */

function mockFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => body,
    text: async () => JSON.stringify(body),
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function adapter() {
  return new RestApiAdapter("http://server");
}

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("billingGetOverview", () => {
  it("maps the server's snake_case payload into the UI shape", async () => {
    mockFetch({
      plan: "pro",
      status: "active",
      credits_total: 2_000_000,
      credits_used: 123_000,
      credits_remaining: 1_877_000,
      spendable_credits: 2_077_000,
      month_resets_at: "2026-08-01T00:00:00Z",
      subscription: {
        workspace_id: "ws-1",
        plan: "pro",
        status: "active",
        seat_count: 3,
        stripe_customer_id: "cus_123",
        trial_ends_at: "2026-07-25T00:00:00Z",
      },
    });

    const overview = await adapter().billingGetOverview("acme");

    expect(overview.subscription.plan).toBe("pro");
    expect(overview.subscription.seatCount).toBe(3);
    expect(overview.subscription.trialEndsAt).toBe("2026-07-25T00:00:00Z");
    // The credit card renders only when this is defined — it never was.
    expect(overview.credits.creditsTotal).toBe(2_000_000);
    expect(overview.credits.creditsUsed).toBe(123_000);
    expect(overview.credits.resetsAt).toBe("2026-08-01T00:00:00Z");
    // The full multi-bucket balance (plan + trial grant + purchased packs).
    expect(overview.spendableCredits).toBe(2_077_000);
    // The Manage Subscription button renders only when this is defined.
    expect(overview.stripeCustomerId).toBe("cus_123");
  });

  it("falls back to the top-level plan when a workspace has no subscription row", async () => {
    // A Free workspace: no monthly plan bucket (Free has no recurring
    // allowance); spendable_credits carries the remaining one-time trial grant.
    mockFetch({
      plan: "free",
      status: "active",
      credits_total: 0,
      credits_used: 0,
      credits_remaining: 0,
      spendable_credits: 200_000,
      month_resets_at: "2026-08-01T00:00:00Z",
      subscription: { workspace_id: "ws-1", plan: "free", status: "active", seat_count: 0 },
    });

    const overview = await adapter().billingGetOverview("acme");

    expect(overview.subscription.plan).toBe("free");
    expect(overview.subscription.seatCount).toBe(1);
    expect(overview.spendableCredits).toBe(200_000);
    expect(overview.stripeCustomerId).toBeUndefined();
  });
});

describe("billingGetUsage", () => {
  it("maps usage_by_operation into the breakdown, and totals everything charged", async () => {
    mockFetch({
      usage_by_operation: {
        ai_translation: 80_000,
        bravo_message: 25_000,
        // An operation the breakdown has no named row for must still be counted.
        brand_scan: 3_683,
      },
    });

    const usage = await adapter().billingGetUsage("acme");

    expect(usage.aiTranslation).toBe(80_000);
    expect(usage.bravoMessages).toBe(25_000);
    expect(usage.aiQualityCheck).toBe(0);
    expect(usage.total).toBe(108_683);
  });
});

describe("billingGetLedger", () => {
  it("reads the entries of the usage response", async () => {
    const fetchMock = mockFetch({
      entries: [
        {
          id: 7,
          amount: -1200,
          balance_after: 498_800,
          operation: "ai_translation",
          reference_id: "job-1",
          created_at: "2026-07-13T10:00:00Z",
        },
      ],
    });

    const ledger = await adapter().billingGetLedger("acme");

    // There is no /billing/ledger route — asking for one 404'd, which is why the
    // Credit Transactions table was always empty.
    expect(fetchMock.mock.calls[0][0]).toContain("/billing/usage");
    expect(ledger).toHaveLength(1);
    expect(ledger[0]).toMatchObject({
      id: "7",
      amount: -1200,
      balanceAfter: 498_800,
      operation: "ai_translation",
      referenceId: "job-1",
      createdAt: "2026-07-13T10:00:00Z",
    });
  });
});

describe("checkout", () => {
  it("sends a plan (never a price) and returns the URL the server named", async () => {
    const fetchMock = mockFetch({ checkout_url: "https://checkout.stripe.com/s/1" });

    const { url } = await adapter().billingCreateCheckout(
      "acme",
      "team",
      "https://ok",
      "https://no",
      5,
    );

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toContain("/billing/checkout");
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body).toMatchObject({ plan: "team", seats: 5 });
    expect(body.price_id).toBeUndefined();
    // The response key is checkout_url; reading `url` from it yielded undefined,
    // so the browser navigated to "undefined".
    expect(url).toBe("https://checkout.stripe.com/s/1");
  });

  it("buys a credit pack", async () => {
    const fetchMock = mockFetch({ checkout_url: "https://checkout.stripe.com/s/2" });

    const { url } = await adapter().billingBuyCredits("acme", "https://ok", "https://no");

    expect(fetchMock.mock.calls[0][0]).toContain("/billing/buy-credits");
    expect(url).toBe("https://checkout.stripe.com/s/2");
  });

  it("returns the portal URL under the key the server sends", async () => {
    mockFetch({ portal_url: "https://billing.stripe.com/p/1" });

    const { url } = await adapter().billingCreatePortal("acme", "https://back");

    expect(url).toBe("https://billing.stripe.com/p/1");
  });
});

import { describe, expect, it, beforeEach } from "vitest";
import { currentAcquisitionSource, stashAcquisitionSource } from "./acquisition";

const COOKIE = "bowrain_acquisition";

function wipeCookies() {
  for (const c of document.cookie.split("; ")) {
    const name = c.split("=")[0];
    if (name) document.cookie = `${name}=; path=/; max-age=0`;
  }
}

function stashedSource(): string | null {
  const raw = document.cookie
    .split("; ")
    .find((c) => c.startsWith(`${COOKIE}=`))
    ?.slice(COOKIE.length + 1);
  return raw ? decodeURIComponent(raw) : null;
}

/** Set the page URL and the referrer the next read will see. */
function arriveAt(search: string, referrer = "") {
  window.history.replaceState({}, "", `/${search}`);
  Object.defineProperty(document, "referrer", { value: referrer, configurable: true });
}

beforeEach(() => {
  wipeCookies();
  arriveAt("");
});

describe("currentAcquisitionSource", () => {
  it("takes the campaign the landing stamped", () => {
    arriveAt("?utm_source=bowrain-landing");
    expect(currentAcquisitionSource()).toBe("bowrain-landing");
  });

  it("prefers the campaign over the referrer", () => {
    arriveAt("?utm_source=newsletter", "https://news.ycombinator.com/item?id=1");
    expect(currentAcquisitionSource()).toBe("newsletter");
  });

  it("falls back to the referring host", () => {
    arriveAt("", "https://news.ycombinator.com/item?id=1");
    expect(currentAcquisitionSource()).toBe("news.ycombinator.com");
  });

  it("ignores a referrer from this same origin — the app talking to itself", () => {
    arriveAt("", `${window.location.origin}/login`);
    expect(currentAcquisitionSource()).toBeNull();
  });

  it("normalizes case", () => {
    arriveAt("?utm_source=Bowrain-Landing");
    expect(currentAcquisitionSource()).toBe("bowrain-landing");
  });

  it("drops a value outside the label shape rather than repairing it", () => {
    arriveAt("?utm_source=two%20words");
    expect(currentAcquisitionSource()).toBeNull();

    arriveAt(`?utm_source=${"a".repeat(65)}`);
    expect(currentAcquisitionSource()).toBeNull();

    arriveAt("?utm_source=%3Cscript%3E");
    expect(currentAcquisitionSource()).toBeNull();
  });

  it("says nothing for a direct visit", () => {
    expect(currentAcquisitionSource()).toBeNull();
  });
});

describe("stashAcquisitionSource", () => {
  it("stashes the label so it survives the OIDC round-trip", () => {
    arriveAt("?utm_source=bowrain-landing");
    stashAcquisitionSource();
    expect(stashedSource()).toBe("bowrain-landing");
  });

  it("leaves an earlier label alone when this visit carries none", () => {
    arriveAt("?utm_source=newsletter");
    stashAcquisitionSource();

    arriveAt("");
    stashAcquisitionSource();
    expect(stashedSource()).toBe("newsletter");
  });

  it("writes nothing at all for a direct first visit", () => {
    stashAcquisitionSource();
    expect(stashedSource()).toBeNull();
  });
});

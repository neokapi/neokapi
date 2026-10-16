import { afterEach, describe, expect, it, vi } from "vitest";
import { interceptLinkClick, linkClickAction } from "../hooks/useAppInit";

describe("what a click on a link does", () => {
  it("opens http, https and mailto links in the system browser", () => {
    expect(linkClickAction("https://bowrain.cloud/docs/")).toEqual({
      kind: "open",
      url: "https://bowrain.cloud/docs/",
    });
    expect(linkClickAction("http://localhost:8080/acme/help")).toEqual({
      kind: "open",
      url: "http://localhost:8080/acme/help",
    });
    expect(linkClickAction("mailto:team@example.com")).toEqual({
      kind: "open",
      url: "mailto:team@example.com",
    });
  });

  it("refuses every other scheme, in any case and with leading space", () => {
    for (const href of [
      "javascript:alert(document.domain)",
      "JavaScript:alert(1)",
      "  javascript:alert(1)",
      "file:///etc/passwd",
      "data:text/html,<script>alert(1)</script>",
      "vbscript:msgbox(1)",
      "kapi://open",
    ]) {
      expect({ href, action: linkClickAction(href) }).toEqual({ href, action: { kind: "refuse" } });
    }
  });

  it("refuses a protocol-relative href, which would load another origin into the webview", () => {
    expect(linkClickAction("//evil.example/page")).toEqual({ kind: "refuse" });
  });

  it("refuses an href that does not parse", () => {
    expect(linkClickAction("docs/getting-started")).toEqual({ kind: "refuse" });
  });

  it("leaves fragments and paths to the app", () => {
    expect(linkClickAction("#section")).toEqual({ kind: "app" });
    expect(linkClickAction("/settings")).toEqual({ kind: "app" });
    expect(linkClickAction("")).toEqual({ kind: "app" });
    expect(linkClickAction(null)).toEqual({ kind: "app" });
  });
});

describe("the document's link intercept", () => {
  const open = vi.fn();
  const handler = (e: MouseEvent) => interceptLinkClick(e, open);

  afterEach(() => {
    document.removeEventListener("click", handler);
    document.body.innerHTML = "";
    open.mockReset();
  });

  /** Click the first element matching `selector` and say whether the click was cancelled. */
  function click(html: string, selector: string): boolean {
    document.body.innerHTML = html;
    document.addEventListener("click", handler);
    const el = document.querySelector(selector)!;
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    el.dispatchEvent(event);
    return event.defaultPrevented;
  }

  it("opens an allowed link in the system browser and keeps the webview where it is", () => {
    const cancelled = click('<a href="https://bowrain.cloud/docs/">About</a>', "a");
    expect(cancelled).toBe(true);
    expect(open).toHaveBeenCalledWith("https://bowrain.cloud/docs/");
  });

  it("finds the link from an icon inside it", () => {
    const cancelled = click(
      '<a href="https://bowrain.cloud/docs/">About <svg><path /></svg></a>',
      "path",
    );
    expect(cancelled).toBe(true);
    expect(open).toHaveBeenCalledWith("https://bowrain.cloud/docs/");
  });

  it("neither opens nor follows a javascript: link", () => {
    const cancelled = click('<a href="javascript:alert(document.domain)">Open</a>', "a");
    expect(cancelled).toBe(true);
    expect(open).not.toHaveBeenCalled();
  });

  it("neither opens nor follows a file: link", () => {
    const cancelled = click('<a href="file:///etc/passwd">Open</a>', "a");
    expect(cancelled).toBe(true);
    expect(open).not.toHaveBeenCalled();
  });

  it("leaves an in-app link and a click outside any link alone", () => {
    expect(click('<a href="#rules">Rules</a>', "a")).toBe(false);
    expect(click("<button>Keep</button>", "button")).toBe(false);
    expect(open).not.toHaveBeenCalled();
  });
});

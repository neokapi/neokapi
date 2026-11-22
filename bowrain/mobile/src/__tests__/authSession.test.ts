import * as WebBrowser from "expo-web-browser";
import type { AuthSessionOpenOptions } from "expo-web-browser";

import { authSessionOptions, endProviderSession, type OpenAuthSession } from "../lib/authSession";

const endSessionUrl =
  "http://localhost:8180/realms/bowrain/protocol/openid-connect/logout?post_logout_redirect_uri=bowrain%3A%2F%2Fauth%2Fsigned-out";
const redirectUri = "bowrain://auth/signed-out";

function recorder(result: Awaited<ReturnType<OpenAuthSession>>) {
  const calls: { url: string; redirect: string; options?: AuthSessionOpenOptions }[] = [];
  const open: OpenAuthSession = async (url, redirect, options) => {
    calls.push({ url, redirect, options });
    return result;
  };
  return { calls, open };
}

describe("auth session", () => {
  it("signs in with an ephemeral session on iOS only", () => {
    expect(authSessionOptions("ios")).toEqual({ preferEphemeralSession: true });
    expect(authSessionOptions("android")).toEqual({});
  });

  it("ends the provider session on Android and waits for the app's redirect", async () => {
    const { calls, open } = recorder({ type: "success", url: redirectUri });
    const res = await endProviderSession({ platform: "android", endSessionUrl, redirectUri, open });
    expect(res).toBe("ended");
    expect(calls).toEqual([{ url: endSessionUrl, redirect: redirectUri, options: {} }]);
  });

  it("opens no browser on iOS, where sign-in kept no provider cookie", async () => {
    const { calls, open } = recorder({ type: "success", url: redirectUri });
    expect(await endProviderSession({ platform: "ios", endSessionUrl, redirectUri, open })).toBe(
      "skipped",
    );
    expect(calls).toHaveLength(0);
  });

  it("opens no browser when the server returned no end-session URL", async () => {
    const { calls, open } = recorder({ type: "success", url: redirectUri });
    expect(await endProviderSession({ platform: "android", redirectUri, open })).toBe("skipped");
    expect(calls).toHaveLength(0);
  });

  it("reports a dismissed or failing browser without throwing", async () => {
    const dismissed = recorder({ type: WebBrowser.WebBrowserResultType.DISMISS });
    expect(
      await endProviderSession({
        platform: "android",
        endSessionUrl,
        redirectUri,
        open: dismissed.open,
      }),
    ).toBe("cancelled");
    const failing: OpenAuthSession = async () => {
      throw new Error("no browser");
    };
    expect(
      await endProviderSession({ platform: "android", endSessionUrl, redirectUri, open: failing }),
    ).toBe("failed");
  });
});

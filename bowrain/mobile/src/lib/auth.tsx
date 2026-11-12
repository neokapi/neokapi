// The session: sign-in through the server, tokens in the platform keychain,
// and one shared API client that refreshes its access token on a 401.
//
// The server brokers the OIDC flow with the workspace's identity provider
// (Keycloak locally, Cognito in production), because the API accepts only the
// server's own tokens. The app opens the server's sign-in route in a browser
// session with a PKCE challenge and a random state; the person signs in at the
// identity provider; the server redirects to bowrain://auth/callback with a
// one-time code and the state. The app checks the state and redeems the code
// with the PKCE verifier by POST, so the tokens arrive only in a response body.
import { createBowrainClient, type BowrainClient } from "@neokapi/bowrain-client";
import { makeRedirectUri } from "expo-auth-session";
import * as Crypto from "expo-crypto";
import * as SecureStore from "expo-secure-store";
import * as WebBrowser from "expo-web-browser";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";

import { readCallback } from "./callback";
import { authRedirectPath, defaultServerUrl } from "./config";
import { createPkce } from "./pkce";

WebBrowser.maybeCompleteAuthSession();

const SESSION_KEY = "bowrain.session";

export interface Session {
  serverUrl: string;
  accessToken: string;
  refreshToken?: string;
  email?: string;
  name?: string;
}

export type SignInResult = { ok: true } | { ok: false; reason: "cancelled" | "failed"; message?: string };

interface AuthState {
  ready: boolean;
  session: Session | null;
  client: BowrainClient;
  signIn: (serverUrl: string) => Promise<SignInResult>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

async function loadSession(): Promise<Session | null> {
  try {
    const raw = await SecureStore.getItemAsync(SESSION_KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    return null;
  }
}

async function saveSession(s: Session | null): Promise<void> {
  if (s) await SecureStore.setItemAsync(SESSION_KEY, JSON.stringify(s));
  else await SecureStore.deleteItemAsync(SESSION_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [session, setSession] = useState<Session | null>(null);
  const sessionRef = useRef<Session | null>(null);
  const refreshing = useRef<Promise<string | null> | null>(null);
  const serverUrl = session?.serverUrl;

  const update = useCallback(async (s: Session | null) => {
    sessionRef.current = s;
    setSession(s);
    await saveSession(s);
  }, []);

  useEffect(() => {
    void loadSession().then((s) => {
      sessionRef.current = s;
      setSession(s);
      setReady(true);
    });
  }, []);

  const client = useMemo(() => {
    // One refresh at a time: concurrent 401s wait for the same rotation,
    // because the server consumes a refresh token on use.
    const refresh = (): Promise<string | null> => {
      if (refreshing.current) return refreshing.current;
      const current = sessionRef.current;
      if (!current?.refreshToken) return Promise.resolve(null);
      const run = createBowrainClient({ baseUrl: current.serverUrl, getAccessToken: () => null })
        .refresh(current.refreshToken)
        .then(async (tokens) => {
          await update({
            ...current,
            accessToken: tokens.access_token,
            refreshToken: tokens.refresh_token ?? current.refreshToken,
          });
          return tokens.access_token;
        })
        .catch(async (err: unknown) => {
          // A refused refresh ends the session; a network failure keeps it.
          if (err instanceof TypeError) return null;
          await update(null);
          return null;
        })
        .finally(() => {
          refreshing.current = null;
        });
      refreshing.current = run;
      return run;
    };
    // The client reads the tokens through the ref, so it is built once per server.
    return createBowrainClient({
      baseUrl: serverUrl ?? defaultServerUrl,
      getAccessToken: () => sessionRef.current?.accessToken ?? null,
      onUnauthorized: refresh,
    });
  }, [update, serverUrl]);

  const signIn = useCallback(
    async (serverUrl: string): Promise<SignInResult> => {
      const base = serverUrl.replace(/\/+$/, "");
      const redirectUri = makeRedirectUri({ scheme: "bowrain", path: authRedirectPath });
      const pkce = await createPkce();
      const state = Crypto.randomUUID();
      const anon = createBowrainClient({ baseUrl: base, getAccessToken: () => null });
      const result = await WebBrowser.openAuthSessionAsync(
        anon.loginUrl(redirectUri, pkce.challenge, state),
        redirectUri,
      );
      if (result.type !== "success") return { ok: false, reason: "cancelled" };
      const cb = readCallback(result.url, state);
      if (!cb.ok) return { ok: false, reason: "failed", message: cb.message };
      // The tokens come only from the response body of the code exchange.
      const tokens = await anon.exchangeCode(cb.code, pkce.verifier, redirectUri);
      const next: Session = {
        serverUrl: base,
        accessToken: tokens.access_token,
        refreshToken: tokens.refresh_token,
      };
      try {
        const me = await createBowrainClient({ baseUrl: base, getAccessToken: () => next.accessToken }).me();
        next.email = me.email;
        next.name = me.name;
      } catch {
        // The name is for display; the session stands without it.
      }
      await update(next);
      return { ok: true };
    },
    [update],
  );

  const signOut = useCallback(async () => {
    try {
      await client.logout();
    } catch {
      // Signing out on the device does not depend on reaching the server.
    }
    await update(null);
  }, [client, update]);

  const value = useMemo(
    () => ({ ready, session, client, signIn, signOut }),
    [ready, session, client, signIn, signOut],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth is used outside AuthProvider");
  return ctx;
}

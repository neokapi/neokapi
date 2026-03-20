import React, { createContext, useContext, useState, useEffect, useCallback } from "react";
import { ApiClient } from "../api/client";
import * as keycloak from "./keycloak";

interface AuthState {
  authenticated: boolean;
  loading: boolean;
  serverUrl: string | null;
  api: ApiClient | null;
  login: (serverUrl: string, keycloakConfig: keycloak.KeycloakConfig) => Promise<boolean>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState>({
  authenticated: false,
  loading: true,
  serverUrl: null,
  api: null,
  login: async () => false,
  logout: async () => {},
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [authenticated, setAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [serverUrl, setServerUrl] = useState<string | null>(null);
  const [api, setApi] = useState<ApiClient | null>(null);

  // Check for stored session on mount.
  useEffect(() => {
    (async () => {
      const hasSession = await keycloak.hasStoredSession();
      if (hasSession) {
        const url = await keycloak.getStoredServerUrl();
        if (url) {
          const client = new ApiClient({
            baseUrl: url,
            getToken: keycloak.getStoredToken,
            onUnauthorized: () => {
              setAuthenticated(false);
              keycloak.clearTokens();
            },
          });
          setApi(client);
          setServerUrl(url);
          setAuthenticated(true);
        }
      }
      setLoading(false);
    })();
  }, []);

  const login = useCallback(
    async (url: string, config: keycloak.KeycloakConfig): Promise<boolean> => {
      const tokens = await keycloak.login(config);
      if (!tokens) return false;

      const client = new ApiClient({
        baseUrl: url,
        getToken: keycloak.getStoredToken,
        onUnauthorized: () => {
          setAuthenticated(false);
          keycloak.clearTokens();
        },
      });

      setApi(client);
      setServerUrl(url);
      setAuthenticated(true);
      return true;
    },
    [],
  );

  const logout = useCallback(async () => {
    await keycloak.clearTokens();
    setApi(null);
    setServerUrl(null);
    setAuthenticated(false);
  }, []);

  return (
    <AuthContext.Provider value={{ authenticated, loading, serverUrl, api, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}

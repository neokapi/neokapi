/**
 * Keycloak PKCE auth flow for React Native / Expo.
 *
 * Uses expo-auth-session for the OAuth2 PKCE flow with Keycloak.
 * Tokens are stored in expo-secure-store for persistence.
 */
import * as AuthSession from "expo-auth-session";
import * as SecureStore from "expo-secure-store";

const TOKEN_KEY = "bowrain_access_token";
const REFRESH_TOKEN_KEY = "bowrain_refresh_token";
const SERVER_URL_KEY = "bowrain_server_url";

export interface KeycloakConfig {
  serverUrl: string;
  keycloakUrl: string;
  realm: string;
  clientId: string;
}

export interface AuthTokens {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

const discovery = (config: KeycloakConfig): AuthSession.DiscoveryDocument => ({
  authorizationEndpoint: `${config.keycloakUrl}/realms/${config.realm}/protocol/openid-connect/auth`,
  tokenEndpoint: `${config.keycloakUrl}/realms/${config.realm}/protocol/openid-connect/token`,
  endSessionEndpoint: `${config.keycloakUrl}/realms/${config.realm}/protocol/openid-connect/logout`,
});

/** Start the PKCE auth flow. Returns tokens on success, null on cancel. */
export async function login(config: KeycloakConfig): Promise<AuthTokens | null> {
  const redirectUri = AuthSession.makeRedirectUri({ scheme: "bowrain" });
  const disc = discovery(config);

  const request = new AuthSession.AuthRequest({
    clientId: config.clientId,
    redirectUri,
    scopes: ["openid", "profile", "email"],
    usePKCE: true,
    responseType: AuthSession.ResponseType.Code,
  });

  const result = await request.promptAsync(disc);

  if (result.type !== "success" || !result.params.code) {
    return null;
  }

  // Exchange code for tokens.
  const tokenResult = await AuthSession.exchangeCodeAsync(
    {
      clientId: config.clientId,
      code: result.params.code,
      redirectUri,
      extraParams: {
        code_verifier: request.codeVerifier!,
      },
    },
    disc,
  );

  const tokens: AuthTokens = {
    accessToken: tokenResult.accessToken,
    refreshToken: tokenResult.refreshToken ?? "",
    expiresIn: tokenResult.expiresIn ?? 3600,
  };

  await saveTokens(tokens, config.serverUrl);
  return tokens;
}

/** Refresh the access token using the stored refresh token. */
export async function refreshTokens(config: KeycloakConfig): Promise<AuthTokens | null> {
  const refreshToken = await SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
  if (!refreshToken) return null;

  const disc = discovery(config);

  try {
    const tokenResult = await AuthSession.refreshAsync(
      {
        clientId: config.clientId,
        refreshToken,
      },
      disc,
    );

    const tokens: AuthTokens = {
      accessToken: tokenResult.accessToken,
      refreshToken: tokenResult.refreshToken ?? refreshToken,
      expiresIn: tokenResult.expiresIn ?? 3600,
    };

    await saveTokens(tokens, config.serverUrl);
    return tokens;
  } catch {
    // Refresh failed — user must re-authenticate.
    await clearTokens();
    return null;
  }
}

/** Save tokens to secure storage. */
async function saveTokens(tokens: AuthTokens, serverUrl: string): Promise<void> {
  await SecureStore.setItemAsync(TOKEN_KEY, tokens.accessToken);
  await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, tokens.refreshToken);
  await SecureStore.setItemAsync(SERVER_URL_KEY, serverUrl);
}

/** Load stored access token. */
export async function getStoredToken(): Promise<string | null> {
  return SecureStore.getItemAsync(TOKEN_KEY);
}

/** Load stored server URL. */
export async function getStoredServerUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(SERVER_URL_KEY);
}

/** Clear all stored auth data. */
export async function clearTokens(): Promise<void> {
  await SecureStore.deleteItemAsync(TOKEN_KEY);
  await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
}

/** Check if there is a stored auth session. */
export async function hasStoredSession(): Promise<boolean> {
  const token = await SecureStore.getItemAsync(TOKEN_KEY);
  return token != null && token.length > 0;
}

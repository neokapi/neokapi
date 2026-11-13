// The server the app talks to by default. A build names it through
// EXPO_PUBLIC_BOWRAIN_SERVER_URL (eas.json, per profile, read in
// app.config.ts). Without one, a development build uses the local stack and a
// release build uses Bowrain cloud, so a store build never points at
// localhost. A person can choose another server from the sign-in screen
// (serverChoice.ts).
import Constants from "expo-constants";

export const cloudServerUrl = "https://app.bowrain.cloud";
export const localServerUrl = "http://localhost:8080";

const configured = Constants.expoConfig?.extra?.serverUrl as string | undefined;

export const defaultServerUrl: string = configured || (__DEV__ ? localServerUrl : cloudServerUrl);

// Where the server sends the browser back after sign-in. The server's
// sign-in route accepts only a bowrain:// or loopback redirect.
export const authRedirectPath = "auth/callback";

// Where the identity provider sends the browser after sign-out. The server
// accepts it as post_logout_redirect_uri, and the identity provider must list
// it too: the local Keycloak realm does (bowrain/docker/keycloak/realm.json),
// and Cognito takes it as one of the app client's sign-out URLs.
export const signedOutRedirectUri = "bowrain://auth/signed-out";

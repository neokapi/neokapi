// The server the app talks to by default, from app.config.ts. A person can
// sign in to another server from the sign-in screen; that choice is stored
// with the session.
import Constants from "expo-constants";

export const defaultServerUrl: string =
  (Constants.expoConfig?.extra?.serverUrl as string | undefined) ?? "http://localhost:8080";

// Where the server sends the browser back after sign-in. The server's
// sign-in route accepts only a bowrain:// or loopback redirect.
export const authRedirectPath = "auth/callback";

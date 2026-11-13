// The app's native configuration. ios/ and android/ are generated from this
// file by `expo prebuild` and are never edited by hand; a native setting that
// is not here belongs in a config plugin.
import type { ConfigContext, ExpoConfig } from "expo/config";

// The Bowrain server the app signs in to and reads from. A development build
// points at the local stack (`make -C bowrain stack-up`); EAS profiles set the
// production URL. On an Android emulator the host machine is 10.0.2.2.
const serverUrl = process.env.EXPO_PUBLIC_BOWRAIN_SERVER_URL ?? "http://localhost:8080";

// The EAS project @neokapi/bowrain, from `eas init`. The config is dynamic, so
// `eas init` prints these instead of writing them here. Both are public
// identifiers; the environment variables override them.
const easProjectId = process.env.EAS_PROJECT_ID ?? "37119e10-8ed7-40c2-b1ea-c1fad898d0b4";
const easOwner = process.env.EXPO_OWNER ?? "neokapi";

// The web domain whose apple-app-site-association file lists this app
// (bowrain/apps/web/public/.well-known/). Universal links and the https
// sign-in redirect resolve against it.
const webDomain = "app.bowrain.cloud";

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: "Bowrain",
  slug: "bowrain",
  owner: easOwner || undefined,
  version: "0.1.0",
  orientation: "portrait",
  icon: "./assets/images/icon.png",
  // The server's sign-in route accepts a bowrain:// redirect.
  scheme: "bowrain",
  userInterfaceStyle: "automatic",
  ios: {
    bundleIdentifier: "cloud.bowrain.app",
    supportsTablet: false,
    associatedDomains: [`applinks:${webDomain}`, `webcredentials:${webDomain}`],
    infoPlist: {
      // The local stack is plain HTTP; production is HTTPS only.
      NSAppTransportSecurity: { NSAllowsLocalNetworking: true },
      ITSAppUsesNonExemptEncryption: false,
    },
  },
  android: {
    package: "cloud.bowrain.app",
    adaptiveIcon: {
      backgroundColor: "#f8fafc",
      foregroundImage: "./assets/images/android-icon-foreground.png",
      backgroundImage: "./assets/images/android-icon-background.png",
      monochromeImage: "./assets/images/android-icon-monochrome.png",
    },
  },
  plugins: [
    "expo-router",
    "expo-secure-store",
    "expo-sqlite",
    "expo-localization",
    "expo-web-browser",
    [
      "expo-splash-screen",
      {
        backgroundColor: "#ffffff",
        image: "./assets/images/splash-icon.png",
        imageWidth: 200,
        dark: { backgroundColor: "#0b0f19" },
      },
    ],
  ],
  experiments: {
    typedRoutes: true,
    reactCompiler: true,
  },
  extra: {
    serverUrl,
    eas: easProjectId ? { projectId: easProjectId } : undefined,
  },
});

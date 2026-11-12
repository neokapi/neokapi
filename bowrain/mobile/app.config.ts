// The app's native configuration. ios/ and android/ are generated from this
// file by `expo prebuild` and are never edited by hand; a native setting that
// is not here belongs in a config plugin.
import type { ConfigContext, ExpoConfig } from "expo/config";

// The Bowrain server the app signs in to and reads from. A development build
// points at the local stack (`make -C bowrain stack-up`); EAS profiles set the
// production URL. On an Android emulator the host machine is 10.0.2.2.
const serverUrl = process.env.EXPO_PUBLIC_BOWRAIN_SERVER_URL ?? "http://localhost:8080";

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: "Bowrain",
  slug: "bowrain",
  owner: process.env.EXPO_OWNER,
  version: "0.1.0",
  orientation: "portrait",
  icon: "./assets/images/icon.png",
  // The server's sign-in route accepts a bowrain:// redirect.
  scheme: "bowrain",
  userInterfaceStyle: "automatic",
  ios: {
    bundleIdentifier: "cloud.bowrain.app",
    supportsTablet: false,
    infoPlist: {
      // The local stack is plain HTTP; production is HTTPS only.
      NSAppTransportSecurity: { NSAllowsLocalNetworking: true },
      ITSAppUsesNonExemptEncryption: false,
    },
  },
  android: {
    package: "cloud.bowrain.app",
    adaptiveIcon: {
      backgroundColor: "#1d4ed8",
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
        imageWidth: 76,
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
    eas: process.env.EAS_PROJECT_ID ? { projectId: process.env.EAS_PROJECT_ID } : undefined,
  },
});

// What the fingerprint runtime version leaves out (app.config.ts sets the
// policy). A value here replaces the preset's list, so it repeats Expo's
// "balanced" skips and adds one: the config's `extra` section. `extra` holds
// the default server URL, read from EXPO_PUBLIC_BOWRAIN_SERVER_URL, and the
// EAS project id. Both are JavaScript data that every update carries in its
// own manifest, so they never change the native app. Hashing them would give
// `eas update` a different runtime version from the build whenever the
// variable is set for one and not the other, and the update would never
// reach the build.
/** @type {import('@expo/fingerprint').Config} */
module.exports = {
  sourceSkips: [
    "PackageJsonAndroidAndIosScriptsIfNotContainRun",
    "ExpoConfigVersions",
    "ExpoConfigRuntimeVersionIfString",
    "EasJson",
    "Easignore",
    "AutolinkingConfigPaths",
    "ExpoConfigExtraSection",
  ],
};

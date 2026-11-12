// Metro for the app inside the pnpm workspace. expo/metro-config detects the
// workspace root and pnpm's isolated layout on its own, so the workspace
// packages the app imports (@neokapi/bowrain-client, @neokapi/kapi-format,
// @neokapi/contract-types) resolve from source through their symlinks.
// Uniwind compiles global.css and must wrap the config last.
const { getDefaultConfig } = require("expo/metro-config");
const { withUniwindConfig } = require("uniwind/metro");

const config = getDefaultConfig(__dirname);

module.exports = withUniwindConfig(config, {
  cssEntryFile: "./src/global.css",
  dtsFile: "./src/uniwind-types.d.ts",
});

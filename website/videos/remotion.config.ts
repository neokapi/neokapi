import { Config } from "@remotion/cli/config";

Config.setVideoImageFormat("jpeg");
Config.setOverwriteOutput(true);
Config.overrideWebpackConfig((config) => ({
  ...config,
  resolve: {
    ...config.resolve,
    fallback: {
      ...config.resolve?.fallback,
      path: false,
      fs: false,
    },
  },
}));

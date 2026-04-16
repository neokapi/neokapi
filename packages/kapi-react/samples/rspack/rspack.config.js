const neokapi = require('@neokapi/kapi-react/webpack'); // Rspack uses webpack-compatible API

/** @type {import('@rspack/core').Configuration} */
module.exports = {
  entry: './src/index.tsx',
  module: {
    rules: [
      {
        test: /\.[jt]sx?$/,
        exclude: /node_modules/,
        loader: 'builtin:swc-loader',
        options: {
          jsc: {
            parser: { syntax: 'typescript', tsx: true },
            transform: { react: { runtime: 'automatic' } },
          },
        },
      },
    ],
  },
  resolve: {
    extensions: ['.tsx', '.ts', '.jsx', '.js'],
  },
  plugins: [
    neokapi({
      locale: process.env.LOCALE,
      translationsDir: './translations',
    }),
  ],
};

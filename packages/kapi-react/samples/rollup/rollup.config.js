import { defineConfig } from 'rollup';
import { nodeResolve } from '@rollup/plugin-node-resolve';
import swc from '@rollup/plugin-swc';
import neokapi from '@neokapi/kapi-react/rollup';

export default defineConfig({
  input: 'src/index.tsx',
  output: {
    dir: 'dist',
    format: 'es',
  },
  plugins: [
    neokapi({
      locale: process.env.LOCALE,
      translationsDir: './translations',
    }),
    nodeResolve({ extensions: ['.tsx', '.ts', '.jsx', '.js'] }),
    swc({
      jsc: {
        parser: { syntax: 'typescript', tsx: true },
        transform: { react: { runtime: 'automatic' } },
      },
    }),
  ],
  external: ['react', 'react-dom', 'react/jsx-runtime'],
});

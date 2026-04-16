import { build } from 'esbuild';
import neokapi from '@neokapi/kapi-react/esbuild';

await build({
  entryPoints: ['src/index.tsx'],
  bundle: true,
  outdir: 'dist',
  format: 'esm',
  jsx: 'automatic',
  plugins: [
    neokapi({
      locale: process.env.LOCALE,
      translationsDir: './translations',
    }),
  ],
  external: ['react', 'react-dom'],
});

console.log('Build complete');

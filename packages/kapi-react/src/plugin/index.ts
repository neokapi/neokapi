/**
 * @neokapi/kapi-react — zero-config i18n for React.
 *
 * Usage:
 *   import neokapi from '@neokapi/kapi-react/vite';
 *   // or: from '@neokapi/kapi-react/webpack'
 *   // or: from '@neokapi/kapi-react/rollup'
 *   // or: from '@neokapi/kapi-react/esbuild'
 */

import { createUnplugin } from 'unplugin';
import { transform } from './transform.ts';
import type { PluginOptions } from '../types.ts';

export type { PluginOptions };

export const unpluginFactory = (options: PluginOptions = {}) => ({
  name: 'neokapi-react',
  enforce: 'pre' as const,

  transformInclude(id: string) {
    return /\.[jt]sx$/.test(id);
  },

  transform(code: string, id: string) {
    // Dev mode: no locale and no runtime mode → no-op
    if (!options.locale && options.mode !== 'runtime') return null;
    return transform(code, id, options);
  },
});

export const unplugin = /* #__PURE__ */ createUnplugin(unpluginFactory);

export default unplugin;

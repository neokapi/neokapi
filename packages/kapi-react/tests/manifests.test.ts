import { describe, expect, it } from 'vitest';
import { parseSync } from '@swc/core';

import {
  collectImports,
  parseManifestFromDTS,
  resolveLibraryComponentMap,
} from '../src/plugin/manifests.ts';

function parse(code: string) {
  return parseSync(code, { syntax: 'typescript', tsx: true });
}

describe('collectImports', () => {
  it('groups non-relative imports by source', () => {
    const mod = parse(`
      import { Root as Tabs, Trigger as TabsTrigger } from '@radix-ui/react-tabs';
      import { Button } from '@/components/ui/button';
      import local from './local';
      import sideEffect from '/abs/path';
    `);
    const imports = collectImports(mod);
    expect(Array.from(imports.keys())).toEqual([
      '@radix-ui/react-tabs',
      '@/components/ui/button',
    ]);
    expect(Object.fromEntries(imports.get('@radix-ui/react-tabs')!)).toEqual({
      Root: 'Tabs',
      Trigger: 'TabsTrigger',
    });
  });
});

describe('parseManifestFromDTS', () => {
  it('derives component → HTML element from RefAttributes<HTMLButtonElement>', () => {
    const dts = `
      export declare const TabsTrigger: React.ForwardRefExoticComponent<
        TabsTriggerProps & React.RefAttributes<HTMLButtonElement>
      >;
      export declare const TabsContent: React.ForwardRefExoticComponent<
        TabsContentProps & React.RefAttributes<HTMLDivElement>
      >;
    `;
    const manifest = parseManifestFromDTS(dts);
    expect(manifest?.components).toEqual({
      TabsTrigger: 'button',
      TabsContent: 'div',
    });
  });

  it('tracks `Trigger = typeof TabsTrigger` aliases', () => {
    const dts = `
      export declare const TabsTrigger: React.ForwardRefExoticComponent<
        TabsTriggerProps & React.RefAttributes<HTMLButtonElement>
      >;
      export declare const Trigger: typeof TabsTrigger;
    `;
    const manifest = parseManifestFromDTS(dts);
    expect(manifest?.aliases).toEqual({ Trigger: 'TabsTrigger' });
  });
});

describe('resolveLibraryComponentMap', () => {
  it('returns an empty map when no libraries ship manifests', () => {
    const mod = parse(`import { foo } from 'package-with-no-manifest-whatsoever';`);
    const map = resolveLibraryComponentMap(mod, process.cwd());
    expect(map).toEqual({});
  });

  it('skips relative imports entirely', () => {
    const mod = parse(`
      import { Foo } from './foo';
      import { Bar } from '../bar';
    `);
    const map = resolveLibraryComponentMap(mod, process.cwd());
    expect(map).toEqual({});
  });
});

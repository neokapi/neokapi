import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";

// Navbar entry for the Neokapi WebAssembly Lab status widget. Client-only and
// code-split: the plugin manager + widget never load during SSR, and the navbar
// pays nothing until hydration (the manager itself pulls no wasm — bridges load
// lazily only when a plugin is actually downloaded).
const LazyStatusWidget = React.lazy(() => import("./StatusWidget"));

export default function KapiStatusNavbarItem(): React.ReactElement {
  return (
    <BrowserOnly>
      {() => (
        <Suspense fallback={null}>
          <LazyStatusWidget />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

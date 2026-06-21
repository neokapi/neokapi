import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";

// Navbar entry for the Neokapi WebAssembly Lab status widget. Client-only and
// code-split: the plugin manager + widget never load during SSR, and the navbar
// pays nothing until hydration (the manager itself pulls no wasm — bridges load
// lazily only when a plugin is actually downloaded).
const LazyStatusWidget = React.lazy(() => import("./StatusWidget"));

export default function KapiStatusNavbarItem({
  mobile,
}: {
  mobile?: boolean;
}): React.ReactElement | null {
  // Docusaurus renders every navbar item twice on mobile: once in the persistent
  // top bar and again inside the hamburger sidebar (with mobile={true}). The
  // widget's absolutely-positioned 340px dropdown panel overflows the narrow
  // sidebar, so we render only in the top bar — the pill stays reachable there.
  if (mobile) {
    return null;
  }
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

import React from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "./config";
import { ChunkSafeSuspense } from "../ChunkErrorBoundary";
import { lazyWithRetry } from "../../lib/chunkReload";

// Lazily import the heavy kit (xterm + wasm boot path) as a SEPARATE async
// chunk. A static `import` — or a synchronous `require()` reachable from the
// statically-imported Root — would fold the whole kit into the main bundle, so
// every page load would ship xterm. React.lazy(() => import(...)) forces a
// split chunk that is fetched only when this component first renders in the
// browser.
const LazyModalWithConfig = lazyWithRetry(async () => {
  const { KapiPlaygroundProvider, KapiModal } = await import("@neokapi/kapi-playground");
  function ModalWithConfig(): React.ReactElement {
    const config = useKapiPlaygroundConfig();
    return (
      <KapiPlaygroundProvider config={config}>
        <KapiModal />
      </KapiPlaygroundProvider>
    );
  }
  return { default: ModalWithConfig };
});

// Mounts the single shared <KapiModal> for the whole site (see Root.tsx).
// Client-only + code-split: the chunk loads in the browser only, and the modal
// defers its WASM boot until first opened.
export default function KapiModalMount(): React.ReactElement {
  return (
    <BrowserOnly>
      {() => (
        <ChunkSafeSuspense fallback={null}>
          <LazyModalWithConfig />
        </ChunkSafeSuspense>
      )}
    </BrowserOnly>
  );
}

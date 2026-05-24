import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "./config";

// Full-bleed embed for the dedicated playground page. The heavy kit is loaded
// as a separate async chunk (see KapiModalMount for why a dynamic import, not
// require/static import). Seeds messages.json so the page works out of the box.
const LazyEmbedWithConfig = React.lazy(async () => {
  const { KapiEmbed } = await import("@neokapi/kapi-playground");
  function EmbedWithConfig(): React.ReactElement {
    const config = useKapiPlaygroundConfig();
    return (
      <KapiEmbed
        wasmExecUrl={config.wasmExecUrl}
        wasmUrl={config.wasmUrl}
        seed={["messages.json"]}
      />
    );
  }
  return { default: EmbedWithConfig };
});

export default function KapiEmbedFullBleed(): React.ReactElement {
  return (
    <BrowserOnly fallback={<p>Loading the in-browser terminal…</p>}>
      {() => (
        <Suspense fallback={<p>Loading the in-browser terminal…</p>}>
          <LazyEmbedWithConfig />
        </Suspense>
      )}
    </BrowserOnly>
  );
}

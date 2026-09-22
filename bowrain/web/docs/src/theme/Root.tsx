import React from "react";

import { DocsChannelBanner } from "@site/src/components/DocsChannel";

// Root wraps the whole app, so the banner renders above the navbar on every
// page. It is server-rendered, so the channel is visible in the static HTML
// and to a reader with scripting off. The stable channel renders nothing here.
export default function Root({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <>
      <DocsChannelBanner />
      {children}
    </>
  );
}

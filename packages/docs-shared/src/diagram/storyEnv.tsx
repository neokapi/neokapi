import React from "react";
import type { Decorator } from "@storybook/react-vite";

/*
  Shared Storybook decorator for the diagram kit.

  The diagram CSS themes off `[data-theme="dark"]` (the attribute Docusaurus sets
  on the page) while Storybook's theme toolbar toggles a `dark` class. This
  decorator bridges the two: it reads the Storybook theme global and mirrors it
  onto a `data-theme` wrapper, then frames the SVG on a page-like surface so it
  reads the way it does in the docs.
*/
export const withDiagramTheme: Decorator = (Story, context) => {
  const dark = context.globals.theme === "dark";
  return (
    <div
      data-theme={dark ? "dark" : "light"}
      style={{
        background: dark ? "#0f1715" : "#ffffff",
        color: dark ? "#f3f7f6" : "#1c1e21",
        padding: "32px 24px",
        minHeight: "100%",
      }}
    >
      <div style={{ maxWidth: 800, margin: "0 auto" }}>
        <Story />
      </div>
    </div>
  );
};

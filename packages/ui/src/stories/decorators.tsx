/**
 * Shared Storybook decorators that wire up the context providers
 * needed by higher-level components (TranslationEditor, ProjectView, etc.).
 */

import React from "react";
import type { Decorator } from "@storybook/react";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import type { Workspace } from "../types/api";
import { createMockAdapter } from "./mock-adapter";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

/**
 * Wraps a story with ApiProvider + WorkspaceProvider + BreadcrumbProvider
 * using an in-memory mock adapter.
 */
export const withProviders: Decorator = (Story) => (
  <ApiProvider adapter={createMockAdapter()}>
    <WorkspaceProvider initialWorkspace={mockWorkspace}>
      <BreadcrumbProvider>
        <Story />
      </BreadcrumbProvider>
    </WorkspaceProvider>
  </ApiProvider>
);

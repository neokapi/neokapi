/**
 * Shared Storybook decorators that wire up the context providers
 * needed by higher-level components (TranslationEditor, ProjectView, etc.).
 */

import React from "react";
import type { Decorator } from "@storybook/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import type { BlockInfo, Workspace } from "../types/api";
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
 * Creates a decorator that wraps stories with ApiProvider + WorkspaceProvider
 * + BreadcrumbProvider. Pass custom blocks to seed the mock adapter.
 */
export function createProvidersDecorator(blocks?: BlockInfo[]): Decorator {
  const adapter = createMockAdapter(blocks);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (Story) => (
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <BreadcrumbProvider>
            <Story />
          </BreadcrumbProvider>
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>
  );
}

/**
 * Default providers decorator using sampleBlocks.
 */
export const withProviders: Decorator = createProvidersDecorator();

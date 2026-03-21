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
import { BravoProvider } from "../context/BravoContext";
import type { BlockInfo, Workspace } from "../types/api";
import type { ApiAdapter } from "../api/adapter";
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
export function createProvidersDecorator(
  blocks?: BlockInfo[],
  overrides?: Partial<ApiAdapter>,
): Decorator {
  const base = createMockAdapter(blocks);
  const adapter = overrides ? { ...base, ...overrides } : base;
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (Story) => (
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <BravoProvider>
            <BreadcrumbProvider>
              <Story />
            </BreadcrumbProvider>
          </BravoProvider>
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>
  );
}

/**
 * Default providers decorator using sampleBlocks.
 */
export const withProviders: Decorator = createProvidersDecorator();

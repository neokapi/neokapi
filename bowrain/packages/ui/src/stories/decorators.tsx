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
import { createMockAdapter, type MockAdapter } from "./mock-adapter";

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
 *
 * The overrides are a `MockAdapter`'s, so a story can seed the mock's own test
 * hooks (`blockEvidence`, `itemNames`) as well as replace a call.
 */
export function createProvidersDecorator(
  blocks?: BlockInfo[],
  overrides?: Partial<MockAdapter>,
): Decorator {
  // Assigned onto the adapter rather than spread into a copy: the mock's own
  // calls close over the object `createMockAdapter` built, so a copy would
  // carry the override while every call kept reading the original.
  const adapter = Object.assign(createMockAdapter(blocks), overrides);
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

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConnectorPanel } from "./ConnectorPanel";
import { withBackend } from "../storybook/backend";
import { qk } from "../lib/queryKeys";

/**
 * ConnectorPanel is a Wails-`Backend` container: it lists the available remote
 * connector types and the active connectors through react-query, where each
 * `queryFn` is a generated binding. The stories pre-seed those query keys via
 * `withBackend`; the per-connector content/status queries stay disabled until a
 * connector is selected (a live-runtime interaction), so the default render is
 * fully covered from the cache. The desktop offers remote/CMS connectors only —
 * local files are kapi's concern.
 */

// The desktop's remote connector catalogue (see CONNECTOR_FIELDS).
const connectorTypes = ["wordpress", "figma", "hubspot"];

const connectors = [
  { id: "conn-wp", name: "Marketing WordPress", type: "wordpress", category: "CMS" },
  { id: "conn-figma", name: "Design System", type: "figma", category: "Design" },
];

const meta: Meta<typeof ConnectorPanel> = {
  title: "Bowrain/ConnectorPanel",
  component: ConnectorPanel,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ConnectorPanel>;

/** Two active connectors; the catalogue is available in the Add dialog. */
export const WithConnectors: Story = {
  decorators: [
    withBackend([
      { queryKey: qk.connectorTypes(), data: connectorTypes },
      { queryKey: qk.connectors(), data: connectors },
    ]),
  ],
};

/** No connectors configured yet — the empty prompt shows. */
export const Empty: Story = {
  decorators: [
    withBackend([
      { queryKey: qk.connectorTypes(), data: connectorTypes },
      { queryKey: qk.connectors(), data: [] },
    ]),
  ],
};

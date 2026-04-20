import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const sidebars: SidebarsConfig = {
  bowrainAd: [
    "README",
    {
      type: "category",
      label: "Foundation",
      collapsed: false,
      items: [
        "001-vision-and-modules",
        "002-authentication-and-workspaces",
        "003-permissions",
      ],
    },
    {
      type: "category",
      label: "Data Layer",
      collapsed: false,
      items: [
        "004-content-store",
        "005-streams",
        "006-graph-concept-storage",
        "007-media-and-blob-storage",
      ],
    },
    {
      type: "category",
      label: "Connectivity",
      collapsed: false,
      items: [
        "008-connector-system",
        "009-sync-protocol",
        "010-bowrain-cli-and-project-model",
        "011-rest-api",
      ],
    },
    {
      type: "category",
      label: "Events & Automation",
      collapsed: false,
      items: [
        "012-distributed-event-bus",
        "013-automation-engine",
        "014-translator-workflow",
      ],
    },
    {
      type: "category",
      label: "Intelligence",
      collapsed: false,
      items: ["015-server-ai-operations", "016-bravo-agent"],
    },
    {
      type: "category",
      label: "Applications",
      collapsed: false,
      items: ["017-bowrain-apps", "018-billing-and-plans"],
    },
  ],
};

export default sidebars;

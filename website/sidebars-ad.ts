import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const sidebars: SidebarsConfig = {
  ad: [
    "README",
    {
      type: "category",
      label: "Foundation",
      collapsed: false,
      items: ["001-vision-and-modules", "002-content-model", "003-identity"],
    },
    {
      type: "category",
      label: "Processing",
      collapsed: false,
      items: [
        "004-processing-engine",
        "005-format-system",
        "006-tool-system",
        "007-plugin-system",
      ],
    },
    {
      type: "category",
      label: "Project Model",
      collapsed: false,
      items: ["008-project-model"],
    },
    {
      type: "category",
      label: "Intelligence",
      collapsed: false,
      items: [
        "009-translation-memory",
        "010-terminology",
        "011-ai-providers",
        "012-mt-providers",
      ],
    },
    {
      type: "category",
      label: "Applications",
      collapsed: false,
      items: ["013-kapi-cli", "014-kapi-desktop"],
    },
    {
      type: "category",
      label: "Cross-Cutting",
      collapsed: false,
      items: ["015-testing-and-documentation", "016-metadata-i18n"],
    },
  ],
};

export default sidebars;

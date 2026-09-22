import ComponentTypes from "@theme-original/NavbarItem/ComponentTypes";
import DocsChannelNavbarItem from "@site/src/components/DocsChannel";

// Register a custom navbar item type so the config can place the stable/next
// channel switch with `{ type: "custom-docsChannel" }`.
export default {
  ...ComponentTypes,
  "custom-docsChannel": DocsChannelNavbarItem,
};

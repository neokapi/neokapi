import ComponentTypes from "@theme-original/NavbarItem/ComponentTypes";
import KapiStatusNavbarItem from "@site/src/components/KapiStatusWidget";
import DocsChannelNavbarItem from "@site/src/components/DocsChannel";

// Register custom navbar item types so the config can place them with
// `{ type: "custom-kapiStatus" }` (the Labs status widget) and
// `{ type: "custom-docsChannel" }` (the stable/next channel switch).
export default {
  ...ComponentTypes,
  "custom-kapiStatus": KapiStatusNavbarItem,
  "custom-docsChannel": DocsChannelNavbarItem,
};

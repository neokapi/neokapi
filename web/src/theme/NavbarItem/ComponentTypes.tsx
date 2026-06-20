import ComponentTypes from "@theme-original/NavbarItem/ComponentTypes";
import KapiStatusNavbarItem from "@site/src/components/KapiStatusWidget";

// Register a custom navbar item type so the Labs status widget can sit in the
// navbar via `{ type: "custom-kapiStatus", position: "right" }` in the config.
export default {
  ...ComponentTypes,
  "custom-kapiStatus": KapiStatusNavbarItem,
};

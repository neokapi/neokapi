import ComponentTypes from "@theme-original/NavbarItem/ComponentTypes";
import KapiStatusNavbarItem from "@site/src/components/KapiStatusWidget";
import SurfaceToggle from "@site/src/components/surface/SurfaceToggle";

// Register custom navbar item types so the Labs status widget and the CLI/Desktop
// surface toggle can sit in the navbar via `{ type: "custom-…", position: "…" }`
// entries in the config. The surface toggle self-hides on pages without dual
// content.
export default {
  ...ComponentTypes,
  "custom-kapiStatus": KapiStatusNavbarItem,
  "custom-surfaceToggle": SurfaceToggle,
};

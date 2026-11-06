import { MapView } from "@neokapi/project-screens";
import { MAP_PENDING } from "@neokapi/project-screens/fixtures";

// The Map over the Fernwell fixture, as a still. Navigation is a no-op and the
// frame around it is inert, so a click lands nowhere rather than somewhere
// half-working. The `.kapi-reference` scope gives the screens package its
// tokens and utilities inside the Docusaurus page.
export default function MapPreviewInner() {
  return (
    <div className="kapi-reference" style={{ pointerEvents: "none" }}>
      <MapView projectName="Fernwell" summary={MAP_PENDING} navigate={() => {}} />
    </div>
  );
}

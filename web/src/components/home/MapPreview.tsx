import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import Link from "@docusaurus/Link";
import Translate from "@docusaurus/Translate";
import styles from "./MapPreview.module.css";

// The hero's picture is the product: the Map a project opens on in Kapi
// Desktop, drawn by the same component the app draws it with
// (@neokapi/project-screens) over the Fernwell fixture its stories use. It is
// a still: nothing opens, and the frame says so. Client-only, because the
// fixture stamps its times at import and the screens package is of no use to
// the server render.
const LazyMap = React.lazy(() => import("./MapPreviewInner"));

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <figure className={styles.figure}>
      <div className={styles.window} aria-hidden="true">
        <div className={styles.titlebar}>
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.dot} />
        </div>
        <div className={styles.viewport}>{children}</div>
        <div className={styles.fade} />
      </div>
      <figcaption className={styles.caption}>
        <Translate id="home.map.caption">Kapi Desktop: the Map a project opens on.</Translate>{" "}
        <Link to="/kapi/desktop/overview">
          <Translate id="home.map.link">What it shows</Translate> &rarr;
        </Link>
      </figcaption>
    </figure>
  );
}

export default function MapPreview() {
  return (
    <BrowserOnly fallback={<Frame>{null}</Frame>}>
      {() => (
        <Frame>
          <Suspense fallback={null}>
            <LazyMap />
          </Suspense>
        </Frame>
      )}
    </BrowserOnly>
  );
}

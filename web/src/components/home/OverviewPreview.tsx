import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import Link from "@docusaurus/Link";
import Translate from "@docusaurus/Translate";
import styles from "./OverviewPreview.module.css";

// The hero's picture is the product: the Overview a project opens on in Kapi
// Desktop, drawn by the same component the app draws it with
// (@neokapi/project-screens) over the Fernwell fixture its stories use. It is
// a still: nothing opens, and the frame says so. Client-only, because the
// fixture stamps its times at import and the screens package is of no use to
// the server render.
const LazyOverview = React.lazy(() => import("./OverviewPreviewInner"));

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
        <Translate id="home.overview.caption">
          Kapi Desktop: the Overview a project opens on.
        </Translate>{" "}
        <Link to="/kapi/desktop/overview">
          <Translate id="home.overview.link">What it shows</Translate> &rarr;
        </Link>
      </figcaption>
    </figure>
  );
}

export default function OverviewPreview() {
  return (
    <BrowserOnly fallback={<Frame>{null}</Frame>}>
      {() => (
        <Frame>
          <Suspense fallback={null}>
            <LazyOverview />
          </Suspense>
        </Frame>
      )}
    </BrowserOnly>
  );
}

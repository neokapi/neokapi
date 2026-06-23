import type { ReactNode } from "react";
import "./PendingMedia.css";

interface PendingMediaProps {
  /** What kind of asset this placeholder stands in for. */
  kind: "video" | "screenshot";
  /**
   * Repo-relative path to the scene script that will produce the asset
   * (e.g. `web/walkthroughs/keep-source-on-brand.scene.yaml`). Rendered as a
   * link to the file on GitHub so a reviewer can read the script.
   */
  scene?: string;
  /** Optional heading override; defaults to a kind-appropriate label. */
  title?: string;
  /** A description of what the finished asset will show. */
  children: ReactNode;
}

const GITHUB_BLOB = "https://github.com/neokapi/neokapi/blob/main/";

// A clearly-marked placeholder for a walkthrough video or screenshot that has a
// written scene script but has not been recorded yet. It deliberately does NOT
// render a fake player or a blank frame — it renders a labelled card describing
// what the asset will show and links its scene script, so the docs read
// honestly while the owner reviews scripts before approving regeneration via the
// harness. Theme-aware via CSS (`[data-theme]`), like ThemedVideo, so it needs
// no React color-mode context.
export default function PendingMedia({ kind, scene, title, children }: PendingMediaProps) {
  const label = title ?? (kind === "video" ? "Walkthrough video" : "Screenshot");
  const verb = kind === "video" ? "Pending recording" : "Pending capture";
  const sceneHref = scene ? GITHUB_BLOB + scene : undefined;
  return (
    <figure className="pending-media" role="group" aria-label={`${label} — ${verb}`}>
      <div className="pending-media__frame">
        <span className="pending-media__badge" aria-hidden="true">
          {kind === "video" ? "▶" : "▣"}
        </span>
        <div className="pending-media__body">
          <div className="pending-media__heading">
            <span className="pending-media__label">{label}</span>
            <span className="pending-media__status">{verb}</span>
          </div>
          <div className="pending-media__desc">{children}</div>
          {sceneHref ? (
            <a className="pending-media__link" href={sceneHref} target="_blank" rel="noreferrer">
              View scene script&nbsp;↗
            </a>
          ) : null}
        </div>
      </div>
    </figure>
  );
}

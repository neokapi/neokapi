import React from "react";
import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { useLocation } from "@docusaurus/router";

import styles from "./styles.module.css";

/**
 * The two documentation channels, as a banner and a navbar switch.
 *
 * Production is built from the `docs/stable` branch, which a GA release moves
 * to its tag, and is served at the site root. Every push to main builds the
 * same site at /next/. docusaurus.config.ts resolves which channel a build
 * belongs to and hands it over in `customFields.docsChannel`, so these
 * components read one object instead of inspecting the base URL.
 *
 * Text here is bare JSX, which this site's build-time transform
 * (plugins/neokapi-i18n.mjs) extracts and translates, interpolations included.
 */

export interface DocsChannelInfo {
  /** Which channel this build serves. */
  channel: "stable" | "next";
  /** Version the channel describes, e.g. "1.3.0-rc1". */
  version: string;
  /** Latest GA version, e.g. "1.2.0". */
  stableVersion: string;
  /** Absolute URL of the stable channel root, with a trailing slash. */
  stableUrl: string;
  /** Absolute URL of the next channel root, with a trailing slash. */
  nextUrl: string;
  /** Site route of the installation page, for the banner's link. */
  installTo: string;
}

export function useDocsChannel(): DocsChannelInfo | null {
  const { siteConfig } = useDocusaurusContext();
  return (siteConfig.customFields?.docsChannel as DocsChannelInfo | undefined) ?? null;
}

/**
 * The same page on the other channel. Both channel roots end in a slash and
 * `baseUrl` does too, so stripping one and appending the other keeps the
 * reader where they were.
 */
function channelHref(targetRoot: string, pathname: string, baseUrl: string): string {
  const rest = pathname.startsWith(baseUrl)
    ? pathname.slice(baseUrl.length)
    : pathname.replace(/^\/+/, "");
  return targetRoot + rest;
}

/**
 * A page present on one channel can be absent from the other, so a click asks
 * for it first and lands on the channel root when the answer is 404. The probe
 * runs only for a same-origin target: a PR preview serves neither channel, and
 * an opaque cross-origin response carries no status to read. Any other failure
 * follows the link the reader clicked.
 */
function switchHandler(href: string, channelRoot: string) {
  return (event: React.MouseEvent<HTMLAnchorElement>): void => {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;

    let target: URL;
    try {
      target = new URL(href, window.location.href);
    } catch {
      return;
    }
    if (target.origin !== window.location.origin) return;

    event.preventDefault();
    void (async () => {
      let destination = target.href;
      try {
        const response = await fetch(target.href, { method: "HEAD" });
        if (!response.ok) destination = channelRoot;
      } catch {
        /* the probe itself failed; follow the link as clicked */
      }
      window.location.href = destination;
    })();
  };
}

export function DocsChannelBanner(): React.ReactElement | null {
  const info = useDocsChannel();
  if (!info || info.channel !== "next") return null;

  return (
    <div className={styles.banner} role="note">
      <span>
        These pages describe kapi {info.version}, the release in progress.{" "}
        <Link to={info.installTo}>Install the beta</Link> to follow them, or read the{" "}
        <a href={info.stableUrl}>documentation for {info.stableVersion}</a>.
      </span>
    </div>
  );
}

interface ChannelLink {
  key: "stable" | "next";
  label: React.ReactNode;
  href: string;
  root: string;
}

export default function DocsChannelNavbarItem({
  mobile,
}: {
  mobile?: boolean;
}): React.ReactElement | null {
  const info = useDocsChannel();
  const { siteConfig } = useDocusaurusContext();
  const { pathname } = useLocation();

  if (!info) return null;

  const stableLabel = <>Stable {info.stableVersion}</>;
  const nextLabel = <>Next</>;
  const links: ChannelLink[] = [
    {
      key: "stable",
      label: stableLabel,
      href: channelHref(info.stableUrl, pathname, siteConfig.baseUrl),
      root: info.stableUrl,
    },
    {
      key: "next",
      label: nextLabel,
      href: channelHref(info.nextUrl, pathname, siteConfig.baseUrl),
      root: info.nextUrl,
    },
  ];

  if (mobile) {
    return (
      <li className="menu__list-item">
        <span className={`menu__link ${styles.mobileHeading}`}>Documentation channel</span>
        <ul className="menu__list">
          {links.map((link) => (
            <li className="menu__list-item" key={link.key}>
              <a
                className={`menu__link ${link.key === info.channel ? "menu__link--active" : ""}`}
                href={link.href}
                onClick={switchHandler(link.href, link.root)}
              >
                {link.label}
              </a>
            </li>
          ))}
        </ul>
      </li>
    );
  }

  return (
    <div className={`navbar__item dropdown dropdown--hoverable dropdown--right ${styles.switch}`}>
      <a
        href={links.find((link) => link.key === info.channel)?.href ?? info.stableUrl}
        className="navbar__link"
        aria-haspopup="true"
        aria-label="Documentation channel"
        onClick={(event) => event.preventDefault()}
      >
        {info.channel === "stable" ? stableLabel : nextLabel}
      </a>
      <ul className="dropdown__menu">
        {links.map((link) => (
          <li key={link.key}>
            <a
              className={`dropdown__link ${
                link.key === info.channel ? "dropdown__link--active" : ""
              }`}
              href={link.href}
              onClick={switchHandler(link.href, link.root)}
            >
              {link.label}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}

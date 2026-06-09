"use client";

import * as React from "react";

/**
 * Theme-scope class for portaled popover content.
 *
 * Radix/Base-UI popovers (dropdown menu, select, popover, combobox) render their
 * content through a Portal to `document.body` — outside any wrapper element.
 * When the host scopes its theme CSS variables (`--popover`, `--background`, …)
 * to `:root`, that's fine: the portaled content still inherits them. But a host
 * that scopes the variables to a *class* (e.g. the docs site limits them to
 * `.kapi-reference` so they don't leak into the surrounding page) leaves the
 * portaled content with no variables — it renders transparent/unstyled.
 *
 * This context lets such a host name a class that the portaled content carries,
 * so the variables resolve on the content element itself regardless of where the
 * Portal lands. The default is empty (no extra class) — apps whose variables live
 * on `:root` need do nothing. Wrap the embedding subtree with
 * {@link PortalThemeProvider} to opt in.
 */
const PortalThemeClassContext = React.createContext<string>("");

export function PortalThemeProvider({
  className,
  children,
}: {
  /** Theme-scope class applied to portaled popover content (e.g. "kapi-reference"). */
  className: string;
  children: React.ReactNode;
}) {
  return (
    <PortalThemeClassContext.Provider value={className}>{children}</PortalThemeClassContext.Provider>
  );
}

/** The active portal theme-scope class, or "" when none is set. */
export function usePortalThemeClass(): string {
  return React.useContext(PortalThemeClassContext);
}

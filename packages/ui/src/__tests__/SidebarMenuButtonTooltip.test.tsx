// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it } from "vitest";

import {
  Sidebar,
  SidebarContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  TooltipProvider,
} from "../index";

// A permanently icon-only rail is rendered with collapsible="none" at icon
// width. Its sidebar state is therefore "expanded" — it cannot collapse — while
// every button in it shows nothing but a glyph.
//
// The tooltip gate keys on that state, so such a rail's tooltips were built and
// then hidden forever: hovering a nav icon in the Bowrain workspace produced no
// label at all.
function IconRail({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider>
      <SidebarProvider>
        <Sidebar collapsible="none">
          <SidebarContent>
            <SidebarMenu>
              <SidebarMenuItem>{children}</SidebarMenuItem>
            </SidebarMenu>
          </SidebarContent>
        </Sidebar>
      </SidebarProvider>
    </TooltipProvider>
  );
}

describe("SidebarMenuButton tooltip", () => {
  // No global auto-cleanup is configured in this package, so renders would
  // otherwise stack up and every query after the first would be ambiguous.
  afterEach(cleanup);

  beforeAll(() => {
    // The sidebar asks whether it is on a mobile viewport; jsdom has no
    // matchMedia. Answering "not mobile" is the desktop rail this is about.
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia;

    // Radix positions the tooltip with a ResizeObserver, which jsdom lacks. A
    // no-op is enough: these tests assert what is shown, not where.
    globalThis.ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  });

  it("shows the label on hover in a rail that can never collapse", async () => {
    render(
      <IconRail>
        <SidebarMenuButton tooltip="Context" tooltipWhenExpanded>
          <svg />
        </SidebarMenuButton>
      </IconRail>,
    );

    await userEvent.hover(screen.getByRole("button"));
    await waitFor(() => {
      expect(screen.getAllByText("Context").length).toBeGreaterThan(0);
    });
  });

  it("is the regression: without the opt-in the same rail shows nothing", async () => {
    render(
      <IconRail>
        <SidebarMenuButton tooltip="Context">
          <svg />
        </SidebarMenuButton>
      </IconRail>,
    );

    await userEvent.hover(screen.getByRole("button"));
    // Radix mounts the content on hover; the gate hides it because the sidebar
    // reports "expanded". This is exactly what a user saw on the icon rail.
    await waitFor(() => {
      const shown = screen.queryAllByText("Context").filter((el) => !el.closest("[hidden]"));
      expect(shown).toHaveLength(0);
    });
    // The button keeps its accessible name either way, so an icon is never
    // nameless to a screen reader even when the visual tooltip is suppressed.
    expect(screen.getByRole("button").getAttribute("aria-label")).toBe("Context");
  });

  it("takes the accessible name from an object tooltip too", () => {
    render(
      <IconRail>
        {/* An object is how a caller passes TooltipContent options; it must not
            silently cost the button its name. */}
        <SidebarMenuButton tooltip={{ children: "Insights" }} tooltipWhenExpanded>
          <svg />
        </SidebarMenuButton>
      </IconRail>,
    );
    expect(screen.getByRole("button").getAttribute("aria-label")).toBe("Insights");
  });
});

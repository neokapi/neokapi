// The guide engine with Kapi Desktop's tours and help topics. It sits above
// the window's content so a tour can open the page each step is on.

import { useMemo, useRef, type ReactNode } from "react";
import { useNeokapi } from "@neokapi/i18n-react/runtime";
import { GuideProvider } from "@neokapi/ui-primitives";
import { openInSystemBrowser } from "../hooks/useAppInit";
import { buildGuides } from "./guides";
import { buildTopics } from "./topics";

/** Where this machine keeps the ids of the tours a person finished. */
export const GUIDE_STORAGE_KEY = "kapi-desktop.guides";

export interface DesktopGuideProps {
  /** Open a view in the active tab, or a global one. */
  navigate: (view: string) => void;
  /** A project tab with a checkout is open. */
  hasProject: boolean;
  /** Open a website link; defaults to the system browser. */
  onOpenUrl?: (url: string) => void;
  children: ReactNode;
}

export function DesktopGuide({
  navigate,
  hasProject,
  onOpenUrl = openInSystemBrowser,
  children,
}: DesktopGuideProps) {
  // A step's before() runs later than the render that built it, so it reads
  // the current navigation through refs rather than the values it was built with.
  const navigateRef = useRef(navigate);
  navigateRef.current = navigate;
  const hasProjectRef = useRef(hasProject);
  hasProjectRef.current = hasProject;
  // The copy is translated when it is built, so it is built again when the
  // language changes.
  const { locale } = useNeokapi();
  const guides = useMemo(
    () =>
      buildGuides({
        navigate: (view) => navigateRef.current(view),
        hasProject: () => hasProjectRef.current,
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [locale],
  );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const topics = useMemo(() => buildTopics(), [locale]);
  return (
    <GuideProvider
      guides={guides}
      topics={topics}
      onOpenUrl={onOpenUrl}
      storageKey={GUIDE_STORAGE_KEY}
    >
      {children}
    </GuideProvider>
  );
}

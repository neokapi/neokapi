"use client";

import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { InfoIcon } from "lucide-react";

import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { useOptionalGuide } from "./GuideProvider";

export interface HelpButtonProps {
  /** The help topic the button opens. */
  topic: string;
  className?: string;
}

/**
 * The info button beside a title: it opens the Help panel on the topic for
 * that page or section. It renders nothing outside a `GuideProvider`, or when
 * the provider has no such topic.
 */
export function HelpButton({ topic, className }: HelpButtonProps) {
  const guide = useOptionalGuide();
  if (!guide?.hasTopic(topic)) return null;
  return (
    <Button
      variant="ghost"
      size="icon-xs"
      data-slot="help-button"
      className={cn("text-muted-foreground hover:text-foreground", className)}
      onClick={() => guide.openHelp(topic)}
      aria-label={t("Help for this page", "info button beside a page title, opens the Help panel")}
    >
      <InfoIcon aria-hidden />
    </Button>
  );
}

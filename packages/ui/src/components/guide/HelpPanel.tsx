"use client";

import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ArrowLeftIcon,
  CheckIcon,
  ChevronRightIcon,
  ExternalLinkIcon,
  SearchIcon,
} from "lucide-react";

import { cn } from "../../lib/utils";
import { SectionHeading } from "../SectionHeading";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Markdown } from "../ui/markdown";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "../ui/sheet";
import { TOPIC_HREF_PREFIX } from "./docs-topics";
import type { Guide, HelpTopic } from "./types";

export interface HelpPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The topic shown, or null for the list. */
  topicId: string | null;
  onTopicChange: (topicId: string | null) => void;
  guides: Guide[];
  topics: HelpTopic[];
  completed: ReadonlySet<string>;
  onStartGuide: (guideId: string) => void;
  onOpenUrl: (url: string) => void;
  /** Runs once the panel has finished closing. */
  onClosed?: () => void;
}

function matches(query: string, ...fields: string[]): boolean {
  const q = query.trim().toLocaleLowerCase();
  if (!q) return true;
  return fields.some((f) => f.toLocaleLowerCase().includes(q));
}

function availability(guide: Guide): { ok: true } | { ok: false; reason: string } {
  try {
    return guide.available?.() ?? { ok: true };
  } catch {
    return { ok: true };
  }
}

/**
 * The Help panel: a sheet from the right with the guides and topics, a filter
 * over both, and a reader for one topic.
 */
export function HelpPanel({
  open,
  onOpenChange,
  topicId,
  onTopicChange,
  guides,
  topics,
  completed,
  onStartGuide,
  onOpenUrl,
  onClosed,
}: HelpPanelProps) {
  const topic = topicId ? topics.find((tp) => tp.id === topicId) : undefined;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        data-slot="help-panel"
        className="w-full gap-0 text-foreground data-[side=right]:w-full data-[side=right]:sm:max-w-[28rem]"
        onCloseAutoFocus={() => onClosed?.()}
      >
        {topic ? (
          <TopicView
            key={topic.id}
            topic={topic}
            onTopicChange={onTopicChange}
            guides={guides}
            completed={completed}
            onBack={() => onTopicChange(null)}
            onStartGuide={onStartGuide}
            onOpenUrl={onOpenUrl}
          />
        ) : (
          <ListView
            open={open}
            guides={guides}
            topics={topics}
            completed={completed}
            onOpenTopic={onTopicChange}
            onStartGuide={onStartGuide}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}

function ListView({
  open,
  guides,
  topics,
  completed,
  onOpenTopic,
  onStartGuide,
}: {
  open: boolean;
  guides: Guide[];
  topics: HelpTopic[];
  completed: ReadonlySet<string>;
  onOpenTopic: (id: string) => void;
  onStartGuide: (id: string) => void;
}) {
  const [query, setQuery] = React.useState("");
  React.useEffect(() => {
    if (open) setQuery("");
  }, [open]);

  const shownGuides = guides.filter((g) => matches(query, g.title, g.summary));
  const shownTopics = topics.filter((tp) => matches(query, tp.title, tp.summary));
  const nothing = shownGuides.length === 0 && shownTopics.length === 0;

  return (
    <>
      <SheetHeader className="gap-3 pr-12">
        <SheetTitle>{t("Help", "help panel title")}</SheetTitle>
        <SheetDescription className="sr-only">
          {t("Guides and help topics", "help panel: accessible description")}
        </SheetDescription>
        <div className="relative">
          <SearchIcon
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("Filter guides and topics", "help panel: filter input placeholder")}
            aria-label={t("Filter guides and topics", "help panel: filter input placeholder")}
            className="pl-8"
          />
        </div>
      </SheetHeader>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-4">
        {nothing && (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("No guides or topics match", "help panel: filter found nothing")}
          </p>
        )}
        {shownGuides.length > 0 && (
          <section className="flex flex-col gap-1" aria-label={t("Guides", "help panel section")}>
            <SectionHeading className="px-2 pb-1">
              {t("Guides", "help panel section")}
            </SectionHeading>
            <ul className="flex flex-col">
              {shownGuides.map((guide) => (
                <GuideRow
                  key={guide.id}
                  guide={guide}
                  done={completed.has(guide.id)}
                  onStart={() => onStartGuide(guide.id)}
                />
              ))}
            </ul>
          </section>
        )}
        {shownTopics.length > 0 && (
          <section className="flex flex-col gap-1" aria-label={t("Topics", "help panel section")}>
            <SectionHeading className="px-2 pb-1">
              {t("Topics", "help panel section")}
            </SectionHeading>
            <ul className="flex flex-col">
              {shownTopics.map((topic) => (
                <li key={topic.id}>
                  <button
                    type="button"
                    data-slot="help-topic-row"
                    onClick={() => onOpenTopic(topic.id)}
                    className="flex w-full min-w-0 cursor-pointer items-center gap-2 rounded-lg px-2 py-2 text-left outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
                  >
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-sm text-foreground">{topic.title}</span>
                      <span className="truncate text-[11px] text-muted-foreground">
                        {topic.summary}
                      </span>
                    </span>
                    <ChevronRightIcon
                      aria-hidden
                      className="size-4 shrink-0 text-muted-foreground"
                    />
                  </button>
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>
    </>
  );
}

function GuideRow({
  guide,
  done,
  onStart,
  className,
}: {
  guide: Guide;
  done: boolean;
  onStart: () => void;
  className?: string;
}) {
  const avail = availability(guide);
  return (
    <li
      data-slot="help-guide-row"
      data-completed={done || undefined}
      className={cn("flex min-w-0 items-center gap-2 rounded-lg px-2 py-2", className)}
    >
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="flex min-w-0 items-center gap-1.5 text-sm text-foreground">
          <span className="truncate">{guide.title}</span>
          {done && (
            <CheckIcon
              className="size-3.5 shrink-0 text-success"
              aria-label={t("Done", "help panel: guide already finished")}
              role="img"
            />
          )}
        </span>
        <span className="truncate text-[11px] text-muted-foreground">
          {avail.ok ? guide.summary : avail.reason}
        </span>
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={!avail.ok}
        onClick={onStart}
        aria-label={t("Start {guide}", "help panel: start a guided tour, with its title", {
          guide: guide.title,
        })}
      >
        {t("Start", "help panel: start a guided tour")}
      </Button>
    </li>
  );
}

function TopicView({
  topic,
  guides,
  completed,
  onBack,
  onStartGuide,
  onOpenUrl,
  onTopicChange,
}: {
  topic: HelpTopic;
  onTopicChange: (topicId: string) => void;
  guides: Guide[];
  completed: ReadonlySet<string>;
  onBack: () => void;
  onStartGuide: (id: string) => void;
  onOpenUrl: (url: string) => void;
}) {
  const related = (topic.guides ?? [])
    .map((id) => guides.find((g) => g.id === id))
    .filter((g): g is Guide => !!g);
  const moreUrl = topic.moreUrl;

  return (
    <>
      <SheetHeader className="flex-row items-center gap-1 pr-12">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onBack}
          aria-label={t("Back to help", "help panel: return from a topic to the list")}
        >
          <ArrowLeftIcon />
        </Button>
        <SheetTitle className="min-w-0 truncate">{topic.title}</SheetTitle>
        <SheetDescription className="sr-only">{topic.summary}</SheetDescription>
      </SheetHeader>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-4">
        <Markdown
          variant="chat"
          className="text-sm"
          onLinkClick={(href) => {
            if (href.startsWith(TOPIC_HREF_PREFIX))
              onTopicChange(href.slice(TOPIC_HREF_PREFIX.length));
            else onOpenUrl(href);
          }}
        >
          {topic.body}
        </Markdown>
        {related.length > 0 && (
          <section className="flex flex-col gap-1" aria-label={t("Guides", "help panel section")}>
            <SectionHeading className="px-2 pb-1">
              {t("Guides", "help panel section")}
            </SectionHeading>
            <ul className="flex flex-col">
              {related.map((guide) => (
                <GuideRow
                  key={guide.id}
                  guide={guide}
                  done={completed.has(guide.id)}
                  onStart={() => onStartGuide(guide.id)}
                />
              ))}
            </ul>
          </section>
        )}
        {moreUrl && (
          <Button
            variant="link"
            className="h-auto self-start px-0"
            onClick={() => onOpenUrl(moreUrl)}
          >
            {t(
              "Open on the website",
              "help panel: link to the same section in its page on the website",
            )}
            <ExternalLinkIcon data-icon="inline-end" aria-hidden />
          </Button>
        )}
      </div>
    </>
  );
}

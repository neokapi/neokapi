// A component running in its own Storybook, full page, showing the reviewer's
// translations in place. The reviewer steps through the component's strings
// with previous and next: the current one is outlined in the running
// component and scrolled into view, and it can be approved or fixed from the
// panel below. A second pair of arrows steps through the component's stories;
// on opening, stories that show none of the strings are passed over.
//
// The story is loaded from the host the collection declares, and the
// container's translations are posted into it (the `@neokapi/i18n-react`
// Storybook decorator renders a dictionary a host posts). The page loads
// nothing beyond the story's own origin. Its one message to the app is the
// list of strings it found, which the app checks against the keys it sent.
import {
  PREVIEW_READY_MESSAGE,
  PREVIEW_TRANSLATIONS_MESSAGE,
  previewDictionary,
  rankStories,
  storiesForSource,
  storyLabel,
  storyURL,
  visibleFragments,
  type ContainerUnit,
  type ProjectScope,
  type ReviewContainer,
  type StoryIndex,
} from "@neokapi/bowrain-client";
import { flattenRuns } from "@neokapi/kapi-format";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { WebView, type WebViewMessageEvent, type WebViewNavigation } from "react-native-webview";
import { useTranslation } from "react-i18next";

import { RunText } from "./RunText";
import { SafeAreaView } from "./SafeArea";

/** The message a story posts with the strings it shows. */
export const PREVIEW_FOUND_MESSAGE = "neokapi-preview-found";

/** How long a story has to show one of the strings before it is passed over. */
export const STORY_SETTLE_MS = 3500;

/** Whether a container can be seen running: a component with a Storybook host. */
export function hasLivePreview(c: ReviewContainer): boolean {
  return c.kind === "component" && c.preview?.kind === "storybook" && !!c.collectionId;
}

/**
 * The stories that render a container's component, ranked, read from its
 * Storybook's index (one query, cached, shared by the context page and the
 * live view). Empty until the index loads, and for a component with no story
 * beside its source file, which then has nothing to show running.
 */
export function useLiveStories(
  container: ReviewContainer | undefined,
  scope: ProjectScope,
  loadIndex?: (collectionId: string) => Promise<StoryIndex>,
) {
  const preview = container?.preview;
  const collectionId = container?.collectionId ?? "";
  const title = container?.title ?? "";
  const index = useQuery({
    queryKey: ["story-index", scope.workspace, scope.project, collectionId],
    queryFn: () => loadIndex!(collectionId),
    enabled: !!loadIndex && !!container && hasLivePreview(container),
    staleTime: 10 * 60_000,
  });
  const stories = useMemo(
    () =>
      index.data && preview
        ? rankStories(storiesForSource(index.data, preview.sourceFile), title)
        : [],
    [index.data, preview, title],
  );
  return { stories, index };
}

/** The script that answers the story's ready message with the dictionary. */
export function previewScript(locale: string, translations: Record<string, string>): string {
  // markKeys asks the runtime to bracket each message with its key
  // (@neokapi/i18n-react runtime, keymarks), which highlightScript reads.
  const msg = JSON.stringify({
    type: PREVIEW_TRANSLATIONS_MESSAGE,
    locale,
    translations,
    markKeys: true,
  });
  const ready = JSON.stringify(PREVIEW_READY_MESSAGE);
  return `(function(){
  var msg = ${msg};
  window.__neokapiPreview = msg;
  if (!window.__neokapiPreviewListening) {
    window.__neokapiPreviewListening = true;
    window.addEventListener("message", function (e) {
      if (e && e.data && e.data.type === ${ready}) window.postMessage(window.__neokapiPreview, "*");
    });
  }
  window.postMessage(msg, "*");
})(); true;`;
}

/**
 * One string to find on the page: its block, the key the runtime renders it
 * under, and the text a reader sees of it, for a Storybook whose runtime does
 * not mark keys.
 */
export interface PreviewTarget {
  key: string;
  messageKey?: string;
  fragments: string[];
}

/** The text of each unit as the running component draws it in a reading. */
export function previewTargets(units: ContainerUnit[], source: boolean): PreviewTarget[] {
  return units
    .map((u) => {
      const text =
        source || !u.target.length
          ? flattenRuns(u.source)
          : (u.runtimeText ?? flattenRuns(u.target));
      return { key: u.blockId, messageKey: u.messageKey, fragments: visibleFragments(text) };
    })
    .filter((t) => t.messageKey || t.fragments.length > 0);
}

/**
 * The script that finds the strings on the page, outlines them (the current
 * one solid, the rest dashed), scrolls the current one into view and tells
 * the app which it found. It runs again as the component renders.
 *
 * A string is found by its message key: the runtime brackets each message it
 * renders with invisible marks, an open mark (U+2064, the key in Unicode tag
 * characters, U+E007F) and a close mark (U+2063), and the element is the one
 * holding both. The text search over `fragments` is the fallback for a
 * Storybook built with a runtime that marks nothing.
 */
export function highlightScript(targets: PreviewTarget[], current: string): string {
  const state = JSON.stringify({ targets, current, found: PREVIEW_FOUND_MESSAGE });
  return `(function(){
  window.__nkHighlight = ${state};
  if (!window.__nkScan) {
    window.__nkScan = function () {
      var s = window.__nkHighlight; if (!s || !document.body) return;
      var marked = document.querySelectorAll("[data-nk-mark]");
      for (var i = 0; i < marked.length; i++) {
        marked[i].style.outline = ""; marked[i].style.outlineOffset = ""; marked[i].removeAttribute("data-nk-mark");
      }
      var nodes = [], w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT), n;
      while ((n = w.nextNode())) if (n.nodeValue && n.nodeValue.trim()) nodes.push(n);
      var byKey = {}, open = [];
      for (var j = 0; j < nodes.length; j++) {
        var v = nodes[j].nodeValue;
        for (var i = 0; i < v.length; i++) {
          var c = v.charCodeAt(i);
          if (c === 0x2064) {
            var key = "", k = i + 1;
            while (k + 1 < v.length && v.charCodeAt(k) === 0xDB40) {
              var lo = v.charCodeAt(k + 1); k += 2;
              if (lo === 0xDC7F) break;
              key += String.fromCharCode(lo - 0xDC00);
            }
            open.push({ key: key, node: nodes[j] }); i = k - 1;
          } else if (c === 0x2063) {
            var top = open.pop();
            if (top && !byKey[top.key]) {
              var r = document.createRange(); r.setStart(top.node, 0); r.setEnd(nodes[j], 0);
              var a = r.commonAncestorContainer;
              byKey[top.key] = a.nodeType === 1 ? a : a.parentElement;
            }
          }
        }
      }
      var unmark = function (t) { return t.replace(/\u2063|\u2064|\uDB40[\uDC20-\uDC7F]/g, "").replace(/s+/g, " "); };
      var found = [], cur = null;
      s.targets.forEach(function (t) {
        var el = (t.messageKey && byKey[t.messageKey]) || null;
        for (var f = 0; f < t.fragments.length && !el; f++) {
          for (var j = 0; j < nodes.length; j++) {
            if (unmark(nodes[j].nodeValue).indexOf(t.fragments[f]) >= 0) { el = nodes[j].parentElement; break; }
          }
        }
        if (!el) return;
        found.push(t.key);
        el.setAttribute("data-nk-mark", "1");
        el.style.outline = t.key === s.current ? "3px solid #2563eb" : "2px dashed rgba(37,99,235,0.45)";
        el.style.outlineOffset = "3px";
        if (t.key === s.current) cur = el;
      });
      if (cur && window.__nkScrolledTo !== s.current) {
        window.__nkScrolledTo = s.current;
        cur.scrollIntoView({ block: "center", inline: "center", behavior: "smooth" });
      }
      var sig = found.join(",");
      if (sig !== window.__nkSent && window.ReactNativeWebView) {
        window.__nkSent = sig;
        window.ReactNativeWebView.postMessage(JSON.stringify({ type: s.found, found: found }));
      }
    };
    var pending = 0;
    new MutationObserver(function () {
      clearTimeout(pending); pending = setTimeout(window.__nkScan, 150);
    }).observe(document.documentElement, { childList: true, subtree: true, characterData: true });
    [300, 1000, 2000, 3000].forEach(function (ms) { setTimeout(window.__nkScan, ms); });
  }
  window.__nkScrolledTo = null;
  window.__nkSent = null;
  window.__nkScan();
})(); true;`;
}

/** The block ids a story says it shows, kept to the ones the app asked for. */
export function readFound(data: string, keys: ReadonlySet<string>): string[] | null {
  try {
    const msg = JSON.parse(data) as { type?: unknown; found?: unknown };
    if (msg?.type !== PREVIEW_FOUND_MESSAGE || !Array.isArray(msg.found)) return null;
    return msg.found.filter((k): k is string => typeof k === "string" && keys.has(k));
  } catch {
    return null;
  }
}

function originOf(url: string): string {
  const m = /^(https?:\/\/[^/]+)/i.exec(url);
  return m ? m[1].toLowerCase() : "";
}

export interface LiveViewProps {
  container: ReviewContainer;
  scope: ProjectScope;
  loadIndex: (collectionId: string) => Promise<StoryIndex>;
  /** The string the view opens at. */
  start: string;
  isDone: (u: ContainerUnit) => boolean;
  onApprove: (u: ContainerUnit) => void;
  onFix: (u: ContainerUnit) => void;
  onClose: () => void;
}

/** The full-page live view: the running component, its stories and its strings. */
export function LiveView({
  container,
  scope,
  loadIndex,
  start,
  isDone,
  onApprove,
  onFix,
  onClose,
}: LiveViewProps) {
  const { t } = useTranslation();
  const preview = container.preview;
  const units = container.units;
  const { stories, index } = useLiveStories(container, scope, loadIndex);

  const [source, setSource] = useState(false);
  const [at, setAt] = useState(() =>
    Math.max(
      0,
      units.findIndex((u) => u.blockId === start),
    ),
  );
  const [storyAt, setStoryAt] = useState(0);
  // Passing over stories that show nothing stops once one shows a string or
  // the reviewer picks a story.
  const [seeking, setSeeking] = useState(true);
  const [found, setFound] = useState<Record<string, string[]>>({});

  const story = stories[Math.min(storyAt, Math.max(0, stories.length - 1))];
  const unit = units[at];
  const keys = useMemo(() => new Set(units.map((u) => u.blockId)), [units]);
  const translations = useMemo(() => (source ? {} : previewDictionary(units)), [source, units]);
  const locale = source ? "en" : container.locale;
  const script = previewScript(locale, translations);
  const highlight = highlightScript(previewTargets(units, source), unit?.blockId ?? "");
  const web = useRef<WebView<object>>(null);

  useEffect(() => {
    web.current?.injectJavaScript(script);
  }, [script]);
  useEffect(() => {
    web.current?.injectJavaScript(highlight);
  }, [highlight]);

  const shown = story ? found[story.id] : undefined;
  const foundRef = useRef(found);
  foundRef.current = found;
  // A story that shows none of the strings once it settles is passed over.
  useEffect(() => {
    if (!seeking || !story) return;
    const timer = setTimeout(() => {
      if (foundRef.current[story.id]?.length) return;
      if (storyAt + 1 < stories.length) setStoryAt(storyAt + 1);
      else setSeeking(false);
    }, STORY_SETTLE_MS);
    return () => clearTimeout(timer);
  }, [seeking, story, storyAt, stories.length]);

  const onMessage = (e: WebViewMessageEvent) => {
    if (!story) return;
    const list = readFound(e.nativeEvent.data, keys);
    if (!list) return;
    setFound((f) => ({ ...f, [story.id]: list }));
    if (list.length) setSeeking(false);
  };
  const pickStory = (i: number) => {
    setSeeking(false);
    setStoryAt(i);
  };

  let body;
  if (index.isPending) {
    body = (
      <View testID="live-loading" className="flex-1 items-center justify-center">
        <ActivityIndicator />
      </View>
    );
  } else if (index.isError || !story || !preview) {
    body = (
      <Text
        testID={index.isError ? "live-unreadable" : "live-no-story"}
        className="text-muted-foreground p-4 text-sm"
      >
        {t(index.isError ? "live.unreadable" : "live.noStory")}
      </Text>
    );
  } else {
    const uri = storyURL(preview.url, story.id);
    const origin = originOf(uri);
    body = (
      <WebView<object>
        key={story.id}
        ref={web}
        testID="live-frame"
        source={{ uri }}
        originWhitelist={[origin]}
        onShouldStartLoadWithRequest={(r: WebViewNavigation) =>
          originOf(r.url) === origin || r.url === "about:blank"
        }
        injectedJavaScriptBeforeContentLoaded={script}
        injectedJavaScript={script + highlight}
        onMessage={onMessage}
        javaScriptCanOpenWindowsAutomatically={false}
        setSupportMultipleWindows={false}
        allowsLinkPreview={false}
        incognito
        style={{ backgroundColor: "white" }}
      />
    );
  }

  let storyNote = "";
  if (story) {
    if (seeking && !shown?.length) storyNote = t("live.looking");
    else if (shown === undefined) storyNote = "";
    else if (shown.length)
      storyNote = t("live.shows", { found: shown.length, count: units.length });
    else storyNote = t("live.showsNone");
  }
  const unitShown = !!unit && !!shown?.includes(unit.blockId);
  const done = unit ? isDone(unit) : false;
  const waiting = !!unit && unit.waiting && !done;

  return (
    <SafeAreaView className="bg-background flex-1">
      <View className="border-border flex-row items-center gap-2 border-b px-2 py-2">
        <Pressable
          testID="live-close"
          accessibilityRole="button"
          accessibilityLabel={t("live.close")}
          onPress={onClose}
          className="h-10 w-10 items-center justify-center"
        >
          <Text className="text-foreground text-xl">✕</Text>
        </Pressable>
        <Text className="text-foreground flex-1 text-base font-semibold" numberOfLines={1}>
          {container.title}
        </Text>
        <View accessibilityRole="tablist" className="bg-muted flex-row rounded-lg p-0.5">
          {[false, true].map((s) => (
            <Pressable
              key={String(s)}
              testID={`live-reading-${s ? "source" : "translation"}`}
              accessibilityRole="tab"
              accessibilityState={{ selected: source === s }}
              onPress={() => setSource(s)}
              className={`rounded-md px-3 py-1 ${source === s ? "bg-background" : ""}`}
            >
              <Text
                className={`text-sm ${source === s ? "text-foreground font-semibold" : "text-muted-foreground"}`}
              >
                {t(`inContext.reading.${s ? "source" : "translation"}`)}
              </Text>
            </Pressable>
          ))}
        </View>
      </View>

      {story ? (
        <Stepper
          testID="live-story"
          prevLabel={t("live.prevStory")}
          nextLabel={t("live.nextStory")}
          canPrev={storyAt > 0}
          canNext={storyAt + 1 < stories.length}
          onPrev={() => pickStory(storyAt - 1)}
          onNext={() => pickStory(storyAt + 1)}
        >
          <Text className="text-foreground text-sm font-semibold" numberOfLines={1}>
            {storyLabel(story)}
          </Text>
          <Text className="text-muted-foreground text-xs" numberOfLines={1}>
            {t("live.storyOf", { n: storyAt + 1, count: stories.length })}
            {storyNote ? ` · ${storyNote}` : ""}
          </Text>
        </Stepper>
      ) : null}

      <View className="flex-1 bg-white">{body}</View>

      {unit ? (
        <View className="border-border bg-background gap-2 border-t px-2 pt-2 pb-3">
          <Stepper
            testID="live-string"
            prevLabel={t("live.prevString")}
            nextLabel={t("live.nextString")}
            canPrev={at > 0}
            canNext={at + 1 < units.length}
            onPrev={() => setAt(at - 1)}
            onNext={() => setAt(at + 1)}
          >
            <Text className="text-muted-foreground text-xs">
              {t("live.stringOf", { n: at + 1, count: units.length })}
              {shown !== undefined && !unitShown ? ` · ${t("live.notShown")}` : ""}
            </Text>
          </Stepper>
          <View testID="live-unit" className="gap-1 px-3">
            <View className="flex-row items-start gap-2">
              <View
                className={`mt-2 h-2 w-2 rounded-full ${waiting ? "bg-corrective" : done ? "bg-approve" : "bg-transparent"}`}
              />
              <View className="flex-1 gap-1">
                <RunText
                  runs={source || !unit.target.length ? unit.source : unit.target}
                  size="lg"
                />
                {!source && unit.target.length ? <RunText runs={unit.source} muted /> : null}
              </View>
            </View>
            {waiting ? (
              <View className="flex-row gap-2 pt-1 pl-4">
                <Pressable
                  testID="live-approve"
                  accessibilityRole="button"
                  onPress={() => {
                    onApprove(unit);
                    if (at + 1 < units.length) setAt(at + 1);
                  }}
                  className="bg-approve/15 rounded-lg px-4 py-2"
                >
                  <Text className="text-foreground text-sm font-semibold">
                    {t("inContext.approve")}
                  </Text>
                </Pressable>
                <Pressable
                  testID="live-fix"
                  accessibilityRole="button"
                  onPress={() => onFix(unit)}
                  className="bg-muted rounded-lg px-4 py-2"
                >
                  <Text className="text-foreground text-sm font-semibold">
                    {t("inContext.fix")}
                  </Text>
                </Pressable>
              </View>
            ) : done ? (
              <Text className="text-muted-foreground pl-4 text-xs">{t("inContext.decided")}</Text>
            ) : null}
          </View>
        </View>
      ) : null}
    </SafeAreaView>
  );
}

function Stepper({
  testID,
  prevLabel,
  nextLabel,
  canPrev,
  canNext,
  onPrev,
  onNext,
  children,
}: {
  testID: string;
  prevLabel: string;
  nextLabel: string;
  canPrev: boolean;
  canNext: boolean;
  onPrev: () => void;
  onNext: () => void;
  children: ReactNode;
}) {
  const arrow = (
    label: string,
    glyph: string,
    enabled: boolean,
    onPress: () => void,
    id: string,
  ) => (
    <Pressable
      testID={`${testID}-${id}`}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled: !enabled }}
      disabled={!enabled}
      onPress={onPress}
      className="h-11 w-11 items-center justify-center"
    >
      <Text
        className={`text-2xl ${enabled ? "text-foreground" : "text-muted-foreground opacity-40"}`}
      >
        {glyph}
      </Text>
    </Pressable>
  );
  return (
    <View testID={testID} className="flex-row items-center">
      {arrow(prevLabel, "‹", canPrev, onPrev, "prev")}
      <View className="flex-1 items-center">{children}</View>
      {arrow(nextLabel, "›", canNext, onNext, "next")}
    </View>
  );
}

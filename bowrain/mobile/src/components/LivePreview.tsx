// A component running in its own Storybook, showing the reviewer's
// translations in place: the heading in its heading, the button label in the
// button, at the width the component lays out to. The story is loaded from
// the host the collection declares, and the container's translations are
// posted into it (the `@neokapi/i18n-react` Storybook decorator renders a
// dictionary a host posts), so the component reads as it will ship if the
// reviewer approves what they hold.
//
// The page loads nothing beyond the story's own origin, and the app exposes
// no bridge to it: the WebView has no message handler, so the page cannot
// call into the app.
import {
  PREVIEW_READY_MESSAGE,
  PREVIEW_TRANSLATIONS_MESSAGE,
  previewDictionary,
  storiesForSource,
  storyLabel,
  storyURL,
  type ProjectScope,
  type ReviewContainer,
  type StoryIndex,
} from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";
import { WebView, type WebViewNavigation } from "react-native-webview";
import { useTranslation } from "react-i18next";

export interface LivePreviewProps {
  container: ReviewContainer;
  scope: ProjectScope;
  loadIndex: (collectionId: string) => Promise<StoryIndex>;
  /** Read the component as authored instead of in the translation. */
  source: boolean;
}

/** Whether a container can be seen running: a component with a Storybook host. */
export function hasLivePreview(c: ReviewContainer): boolean {
  return c.kind === "component" && c.preview?.kind === "storybook" && !!c.collectionId;
}

/** The script that answers the story's ready message with the dictionary. */
export function previewScript(locale: string, translations: Record<string, string>): string {
  const msg = JSON.stringify({ type: PREVIEW_TRANSLATIONS_MESSAGE, locale, translations });
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

function originOf(url: string): string {
  const m = /^(https?:\/\/[^/]+)/i.exec(url);
  return m ? m[1].toLowerCase() : "";
}

export function LivePreview({ container, scope, loadIndex, source }: LivePreviewProps) {
  const { t } = useTranslation();
  const preview = container.preview;
  const collectionId = container.collectionId ?? "";
  const index = useQuery({
    queryKey: ["story-index", scope.workspace, scope.project, collectionId],
    queryFn: () => loadIndex(collectionId),
    enabled: !!preview && !!collectionId,
    staleTime: 10 * 60_000,
  });
  const stories = useMemo(
    () => (index.data && preview ? storiesForSource(index.data, preview.sourceFile) : []),
    [index.data, preview],
  );
  const [picked, setPicked] = useState<string | undefined>();
  const story = stories.find((s) => s.id === picked) ?? stories[0];

  const translations = useMemo(
    () => (source ? {} : previewDictionary(container.units)),
    [source, container.units],
  );
  const locale = source ? "en" : container.locale;
  const script = previewScript(locale, translations);
  // WebView's props are `WebViewProps & P`; the default P of undefined makes
  // them never, so the element names an empty extension.
  const web = useRef<WebView<object>>(null);
  // A change of reading re-posts without reloading the story.
  useEffect(() => {
    web.current?.injectJavaScript(script);
  }, [script]);

  if (!preview) return null;
  if (index.isPending) {
    return (
      <View testID="live-loading" className="h-24 items-center justify-center">
        <ActivityIndicator />
      </View>
    );
  }
  if (index.isError) {
    return (
      <Text testID="live-unreadable" className="text-muted-foreground px-1 text-sm">
        {t("live.unreadable")}
      </Text>
    );
  }
  if (!story) {
    return (
      <Text testID="live-no-story" className="text-muted-foreground px-1 text-sm">
        {t("live.noStory")}
      </Text>
    );
  }
  const uri = storyURL(preview.url, story.id);
  const origin = originOf(uri);
  return (
    <View testID="live-preview" className="gap-2">
      {stories.length > 1 ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerClassName="gap-2"
        >
          {stories.map((s) => (
            <Pressable
              key={s.id}
              testID={`live-story-${s.id}`}
              accessibilityRole="button"
              accessibilityState={{ selected: s.id === story.id }}
              onPress={() => setPicked(s.id)}
              className={`rounded-full border px-3 py-1 ${s.id === story.id ? "border-primary" : "border-border"}`}
            >
              <Text className="text-foreground text-xs">{storyLabel(s)}</Text>
            </Pressable>
          ))}
        </ScrollView>
      ) : (
        <Text className="text-muted-foreground text-xs">{storyLabel(story)}</Text>
      )}
      <View className="border-border h-80 overflow-hidden rounded-xl border bg-white">
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
          injectedJavaScript={script}
          javaScriptCanOpenWindowsAutomatically={false}
          setSupportMultipleWindows={false}
          allowsLinkPreview={false}
          incognito
        />
      </View>
    </View>
  );
}

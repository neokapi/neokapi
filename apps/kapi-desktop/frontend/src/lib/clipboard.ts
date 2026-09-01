/**
 * Write text to the system clipboard.
 *
 * navigator.clipboard.writeText is unreliable inside Wails' webview: the
 * custom wails:// scheme doesn't consistently satisfy the Clipboard API's
 * secure-context/permission checks, so a write can silently fail — or copy
 * stale/partial content — even from a genuine click handler. @wailsio/runtime
 * ships its own Clipboard.SetText, which goes through the Go backend instead
 * and doesn't have this problem, so it's preferred whenever available.
 *
 * Like the rest of the runtime it has load-time side effects outside a Wails
 * window (see platform.ts), so it's imported lazily and resolved once;
 * outside a Wails window (Storybook, the web demo, tests without the mock)
 * it falls back to the browser API.
 */
type WailsClipboard = { SetText: (text: string) => Promise<void> };
let clipboardModule: WailsClipboard | null = null;

const clipboardReady: Promise<WailsClipboard | null> = import("@wailsio/runtime")
  .then((mod) => {
    clipboardModule = mod.Clipboard;
    return clipboardModule;
  })
  .catch(() => null);

function getClipboard(): Promise<WailsClipboard | null> {
  if (clipboardModule) return Promise.resolve(clipboardModule);
  return clipboardReady;
}

export async function writeClipboardText(text: string): Promise<void> {
  const clipboard = await getClipboard();
  if (clipboard) {
    await clipboard.SetText(text);
    return;
  }
  await navigator.clipboard.writeText(text);
}

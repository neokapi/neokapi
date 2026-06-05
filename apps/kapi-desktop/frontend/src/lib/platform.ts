/**
 * Platform detection for desktop window-chrome adjustments.
 *
 * Reads the OS that Wails injects into `window._wails.environment` at startup —
 * the same source `@wailsio/runtime`'s `System.IsMac()` / `IsWindows()` read —
 * but WITHOUT importing the runtime. A static `@wailsio/runtime` import has
 * load-time side effects (it assigns `window._wails.invoke` and emits
 * `wails:runtime:ready`) that throw outside a Wails window, so the rest of the
 * app only imports it lazily. The injected value is present synchronously before
 * first paint, so layout can depend on it flash-free.
 *
 * Outside a Wails window (`window._wails` undefined — Storybook, the web demo)
 * the OS is unknown and these return false, which collapses the macOS-only
 * titlebar insets. That is the right default for those non-native contexts.
 */
interface WailsEnvWindow {
  _wails?: { environment?: { OS?: string; Arch?: string } };
}

function platformOS(): string | undefined {
  return (window as unknown as WailsEnvWindow)._wails?.environment?.OS;
}

/** True only when positively running on macOS inside a Wails window. */
export function isMacDesktop(): boolean {
  return platformOS() === "darwin";
}

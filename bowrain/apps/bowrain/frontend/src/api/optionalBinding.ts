/**
 * Resolve an optional Wails binding by name at runtime.
 *
 * The generated bindings (`bindings/.../app.js`) only export the methods the
 * current desktop backend actually implements. A few adapter/interface methods
 * reference bindings that a given backend build may not provide — either
 * forward-looking ones (e.g. `GetToolSchema`) or capabilities that only exist
 * server-side (e.g. `GetTranslationDashboard`). A static `Backend.Foo` member
 * access on the namespace import makes the bundler reject the build at link
 * time (`IMPORT_IS_UNDEFINED`) because the export is genuinely absent.
 *
 * Looking the binding up by a runtime *name* keeps the "is this available?"
 * check intact while presenting it to the bundler as an ordinary dynamic
 * property read rather than a missing namespace import.
 */
export function optionalBinding<F>(ns: unknown, name: string): F | undefined {
  const fn = (ns as Record<string, unknown>)[name];
  return typeof fn === "function" ? (fn as F) : undefined;
}

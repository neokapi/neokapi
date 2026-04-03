import { useMemo } from "react";
import { renderToStaticMarkup } from "react-dom/server";

/**
 * Renders a React Email component to static HTML and displays it in an
 * iframe. This avoids hydration errors from nesting <html> inside
 * Storybook's DOM, and shows the email as it would appear in a mail client.
 */
export function EmailPreview({ children }: { children: React.ReactElement }) {
  const html = useMemo(() => {
    try {
      return renderToStaticMarkup(children);
    } catch {
      return "<p>Failed to render email</p>";
    }
  }, [children]);

  return (
    <iframe
      srcDoc={`<!DOCTYPE html>${html}`}
      title="Email Preview"
      style={{
        width: "100%",
        height: "800px",
        border: "1px solid #e5e7eb",
        borderRadius: 8,
        background: "#fff",
      }}
    />
  );
}

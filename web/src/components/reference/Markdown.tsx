import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import styles from "./styles.module.css";

/**
 * Render a markdown string from the reference dataset (overviews, parameter
 * help, example descriptions). Kept deliberately small — block elements only,
 * scoped styling lives in the CSS module.
 */
export default function Markdown({ children }: { children?: string }) {
  if (!children) return null;
  return (
    <div className={styles.markdown}>
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{children}</ReactMarkdown>
    </div>
  );
}

/**
 * Example `config` snippets arrive either as raw YAML or wrapped in a fenced
 * code block (```yaml … ```). Normalize to the inner YAML text.
 */
export function unfence(config: string | undefined): string {
  if (!config) return "";
  const trimmed = config.trim();
  const fence = trimmed.match(/^```[a-zA-Z]*\n([\s\S]*?)\n?```$/);
  if (fence) return fence[1].trimEnd();
  return trimmed;
}

/** First non-empty line of a markdown overview, stripped of markdown emphasis. */
export function firstLine(text: string | undefined): string {
  if (!text) return "";
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    return line.replace(/\*\*|__|`/g, "");
  }
  return "";
}

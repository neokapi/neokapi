import useBaseUrl from "@docusaurus/useBaseUrl";
import type { CommandEntry } from "@neokapi/reference-data";
import { commandName, commandSummary } from "./commandHelpers";
import styles from "./styles.module.css";

interface Props {
  cmd: CommandEntry;
  /**
   * Canonical static page route for this command (without baseUrl), e.g.
   * "/reference/commands/pseudo-translate". The card is a real link to it for
   * SEO + open-in-new-tab; a plain left-click opens the quick modal.
   */
  href: string;
  /** Opens the detail modal for this command (and writes ?id= to the URL). */
  onSelect: (id: string) => void;
}

/**
 * Capability badge shown on the card and in the modal header.
 *
 * Three variants:
 *   runnableInBrowser && !demoMode → green "Run"
 *   runnableInBrowser && demoMode  → amber "Demo"
 *   !runnableInBrowser             → purple "Watch"
 */
export function RunBadge({ cmd }: { cmd: Pick<CommandEntry, "runnableInBrowser" | "demoMode"> }) {
  if (cmd.runnableInBrowser && cmd.demoMode) {
    return (
      <span
        className={`${styles.runBadge} ${styles.runDemo}`}
        title="Runs in your browser via a built-in stub — illustrative output, not a real model"
      >
        Demo
      </span>
    );
  }
  if (cmd.runnableInBrowser) {
    return (
      <span className={`${styles.runBadge} ${styles.runOffline}`} title="Runs in your browser">
        Run
      </span>
    );
  }
  return (
    <span
      className={`${styles.runBadge} ${styles.runNetwork}`}
      title="Runs outside the browser — watch a walkthrough instead"
    >
      Watch
    </span>
  );
}

/**
 * A compact card in the command grid. It is a real link to the command's
 * static, shareable page (SEO + open-in-new-tab); a plain left-click instead
 * opens the full {@link CommandModal} quick view, where the detail body and any
 * runnable snippet live, so the grid stays cheap to render across all commands.
 */
export default function CommandCard({ cmd, href, onSelect }: Props) {
  const flagCount = cmd.flags?.length ?? 0;
  const summary = commandSummary(cmd);
  const resolvedHref = useBaseUrl(href);

  return (
    <a
      className={styles.gridCard}
      href={resolvedHref}
      onClick={(e) => {
        if (
          e.defaultPrevented ||
          e.button !== 0 ||
          e.metaKey ||
          e.ctrlKey ||
          e.shiftKey ||
          e.altKey
        )
          return;
        e.preventDefault();
        onSelect(cmd.id);
      }}
    >
      <span className={styles.gridCardHead}>
        <span className={styles.gridCardName}>{commandName(cmd)}</span>
        <RunBadge cmd={cmd} />
      </span>

      {summary && <span className={styles.gridCardDesc}>{summary}</span>}

      <span className={styles.gridCardFoot}>
        {cmd.aliases?.slice(0, 2).map((a) => (
          <span key={a} className={styles.aliasTag}>
            {a}
          </span>
        ))}
        {flagCount > 0 && (
          <span className={styles.gridFlagCount}>
            {flagCount} flag{flagCount !== 1 ? "s" : ""}
          </span>
        )}
      </span>
    </a>
  );
}

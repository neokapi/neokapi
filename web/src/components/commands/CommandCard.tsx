import Link from "@docusaurus/Link";
import type { CommandEntry } from "@neokapi/reference-data";
import { commandName, commandSummary } from "./commandHelpers";
import styles from "./styles.module.css";

interface Props {
  cmd: CommandEntry;
  /**
   * Canonical static page route for this command (without baseUrl), e.g.
   * "/reference/commands/pseudo-translate". The card is a real link to the
   * command's static, shareable reference page.
   */
  href: string;
}

/**
 * Capability badge shown on the card and the command reference page.
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
 * static, shareable reference page (SEO + open-in-new-tab), where the detail
 * body and any runnable snippet live, so the grid stays cheap to render across
 * all commands.
 */
export default function CommandCard({ cmd, href }: Props) {
  const flagCount = cmd.flags?.length ?? 0;
  const summary = commandSummary(cmd);

  return (
    <Link className={styles.gridCard} to={href}>
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
    </Link>
  );
}

import type { CommandEntry } from "@neokapi/reference-data";
import { commandName, commandSummary } from "./commandHelpers";
import styles from "./styles.module.css";

interface Props {
  cmd: CommandEntry;
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
      title="Needs network or a running server — watch a walkthrough instead"
    >
      Watch
    </span>
  );
}

/**
 * A compact, clickable card in the command grid. Clicking opens the full
 * {@link CommandModal}; the detail body and any runnable snippet live there, so
 * the grid stays cheap to render across all commands.
 */
export default function CommandCard({ cmd, onSelect }: Props) {
  const flagCount = cmd.flags?.length ?? 0;
  const summary = commandSummary(cmd);

  return (
    <button
      type="button"
      className={styles.gridCard}
      onClick={() => onSelect(cmd.id)}
      aria-haspopup="dialog"
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
    </button>
  );
}

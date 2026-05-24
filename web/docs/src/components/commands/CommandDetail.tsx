import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { CommandEntry } from "@neokapi/reference-data";
import RunnableSnippet from "@site/src/components/KapiPlayground/RunnableSnippet";
import { commandName, commandSummary, firstRunnableExample, seedFor } from "./commandHelpers";
import styles from "./styles.module.css";

interface Props {
  cmd: CommandEntry;
}

/**
 * The full reference body for one command: synopsis, description, a runnable
 * snippet or a network/watch note, a flags table, and any authored examples.
 * Rendered inside {@link CommandModal}; kept separate so the card grid stays
 * lightweight.
 *
 * Three states:
 *   runnableInBrowser && !demoMode → primary green ▸ Run
 *   runnableInBrowser && demoMode  → amber ▸ Run (demo) + honesty note
 *   !runnableInBrowser             → Watch note (network/server required)
 */
export default function CommandDetail({ cmd }: Props) {
  const name = commandName(cmd);
  const summary = commandSummary(cmd);
  const flags = cmd.flags ?? [];

  const canRun = cmd.runnableInBrowser;
  const isDemo = cmd.demoMode;

  // Prefer the first authored example (already a complete invocation); fall
  // back to a synthesized "kapi <name>" form.
  const firstExample = firstRunnableExample(cmd);
  const runCmd = firstExample ?? `kapi ${commandName(cmd)}`;
  const seed = seedFor(cmd, runCmd);

  // Additional examples beyond the first (shown as a separate list below).
  const remainingExamples = (cmd.examples ?? []).slice(firstExample != null ? 1 : 0);

  return (
    <div className={styles.body}>
      {/* Synopsis (cobra `use`, prefixed with the binary name) */}
      <code className={styles.synopsis}>
        <span className={styles.synopsisPrompt} aria-hidden="true">
          $
        </span>{" "}
        kapi {cmd.use}
      </code>

      {/* Description: long body when present, else the one-line summary */}
      {cmd.long ? (
        <div className={styles.markdown}>
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{cmd.long}</ReactMarkdown>
        </div>
      ) : (
        summary && <p className={styles.description}>{summary}</p>
      )}

      {/* Metadata */}
      <div className={styles.metaGrid}>
        <Meta label="Command" value={name} mono />
        {cmd.groupID && <Meta label="Group" value={cmd.groupID} />}
        {cmd.aliases && cmd.aliases.length > 0 && (
          <Meta label="Aliases" value={cmd.aliases.join(", ")} mono />
        )}
      </div>

      {/* Run / Run (demo) / Watch */}
      <div className={styles.docSection}>
        <div className={styles.sectionTitle}>
          {canRun ? (isDemo ? "Try it (demo)" : "Try it") : "Watch it"}
        </div>
        {canRun ? (
          <>
            {isDemo ? (
              <p className={styles.demoNote}>
                Demo mode — illustrative output from a built-in stub, not a real model. Install the
                CLI to run with your own API key.
              </p>
            ) : (
              <p className={styles.runHint}>
                Runs in your browser against a small sample file. Edit the command before running,
                or press Run to execute it as shown.
              </p>
            )}
            <RunnableSnippet cmd={runCmd} seed={seed.length > 0 ? seed : undefined} editable />
          </>
        ) : (
          <div className={styles.networkNote}>
            <span>
              <strong>{name}</strong> needs network access, an API key, or local system access, so
              it can't run in the browser playground.
            </span>
            <span>
              See the <a href="/walkthroughs">walkthroughs</a> for a recorded run, or try it locally
              with the <a href="/getting-started/installation">installed CLI</a>.
            </span>
          </div>
        )}
      </div>

      {/* Flags */}
      <div className={styles.docSection}>
        <div className={styles.sectionTitle}>Flags</div>
        {flags.length === 0 ? (
          <p className={styles.noFlags}>This command has no local flags.</p>
        ) : (
          <table className={styles.flagTable}>
            <thead>
              <tr>
                <th>Flag</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              {flags.map((f) => (
                <tr key={f.name}>
                  <td className={styles.flagName}>
                    --{f.name}
                    {f.shorthand ? `, -${f.shorthand}` : ""}
                  </td>
                  <td className={styles.flagType}>{f.type ?? ""}</td>
                  <td className={styles.flagDefault}>{formatDefault(f.default, f.type)}</td>
                  <td className={styles.flagUsage}>{f.usage ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Additional examples beyond the first (the first is already the Run snippet) */}
      {remainingExamples.length > 0 && (
        <div className={styles.docSection}>
          <div className={styles.sectionTitle}>More examples</div>
          <div className={styles.exampleList}>
            {remainingExamples.map((ex, i) =>
              canRun && ex.trim().startsWith("kapi") ? (
                <RunnableSnippet
                  key={i}
                  cmd={ex.trim()}
                  seed={(() => {
                    const s = seedFor(cmd, ex.trim());
                    return s.length > 0 ? s : undefined;
                  })()}
                  editable
                />
              ) : (
                <code key={i} className={styles.synopsis}>
                  {ex.trim()}
                </code>
              ),
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/** Render a flag default, collapsing the empty-default and slice cases. */
function formatDefault(def: string | undefined, type: string | undefined): string {
  if (def === undefined || def === "") return "";
  // pflag renders empty slices as "[]"; that is noise in a reference table.
  if ((type === "stringSlice" || type === "stringArray") && def === "[]") return "";
  return def;
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className={styles.metaItem}>
      <span className={styles.metaLabel}>{label}</span>
      <span className={mono ? styles.metaValueMono : styles.metaValue}>{value}</span>
    </div>
  );
}

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { commands as commandData } from "@neokapi/reference-data";
import type { CommandEntry } from "@neokapi/reference-data";
import RunnableSnippet from "@site/src/components/KapiPlayground/RunnableSnippet";
import {
  commandName,
  commandSummary,
  firstRunnableExample,
  seedFor,
} from "@site/src/components/commands/commandHelpers";
import detailStyles from "@site/src/components/commands/styles.module.css";
import styles from "./pages.module.css";

interface Props {
  /** Command id (dot-joined path), e.g. "formats.info". */
  id: string;
}

/** Render a flag default, collapsing the empty-default and slice cases. */
function formatDefault(def: string | undefined, type: string | undefined): string {
  if (def === undefined || def === "") return "";
  if ((type === "stringSlice" || type === "stringArray") && def === "[]") return "";
  return def;
}

/**
 * The full, static reference body for one kapi CLI command, rendered from the
 * generated `@neokapi/reference-data` dataset — synopsis, description, a runnable
 * example (when the command runs in the browser) or a watch note, a flags table,
 * and any additional examples. Imported by the generated MDX page; all content
 * derives from the data so the page is never hand-authored prose.
 */
export default function CommandReferencePage({ id }: Props) {
  const guidesHref = useBaseUrl("/kapi/recipes");
  const installHref = useBaseUrl("/kapi/get-started/installation");
  const commandsHref = useBaseUrl("/commands");
  const cmd: CommandEntry | undefined = commandData.commands.find((c) => c.id === id);
  if (!cmd) {
    return <p>Unknown command: {id}</p>;
  }

  const name = commandName(cmd);
  const summary = commandSummary(cmd);
  const flags = cmd.flags ?? [];
  const canRun = cmd.runnableInBrowser;
  const isDemo = cmd.demoMode;

  const firstExample = firstRunnableExample(cmd);
  const runCmd = firstExample ?? `kapi ${name}`;
  const seed = seedFor(cmd, runCmd);
  const remainingExamples = (cmd.examples ?? []).slice(firstExample != null ? 1 : 0);

  return (
    <div className={styles.page}>
      {/* Synopsis */}
      <code className={detailStyles.synopsis}>
        <span className={detailStyles.synopsisPrompt} aria-hidden="true">
          $
        </span>{" "}
        kapi {cmd.use}
      </code>

      {/* Description */}
      {cmd.long ? (
        <div className={detailStyles.markdown}>
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{cmd.long}</ReactMarkdown>
        </div>
      ) : (
        summary && <p className={detailStyles.description}>{summary}</p>
      )}

      {/* Metadata */}
      <div className={detailStyles.metaGrid}>
        <Meta label="Command" value={name} mono />
        {cmd.groupID && <Meta label="Group" value={cmd.groupID} />}
        {cmd.aliases && cmd.aliases.length > 0 && (
          <Meta label="Aliases" value={cmd.aliases.join(", ")} mono />
        )}
      </div>

      {/* Run / Run (demo) / Watch */}
      <div className={detailStyles.docSection}>
        <div className={detailStyles.sectionTitle}>
          {canRun ? (isDemo ? "Try it (demo)" : "Try it") : "Watch it"}
        </div>
        {canRun ? (
          <>
            {isDemo ? (
              <p className={detailStyles.demoNote}>
                Demo mode — illustrative output from a built-in stub, not a real model. Install the
                CLI to run with your own API key.
              </p>
            ) : (
              <p className={detailStyles.runHint}>
                Runs in your browser against a small sample file. Edit the command before running,
                or press Run to execute it as shown.
              </p>
            )}
            <RunnableSnippet cmd={runCmd} seed={seed.length > 0 ? seed : undefined} editable />
          </>
        ) : (
          <div className={detailStyles.networkNote}>
            <span>
              <strong>{name}</strong> needs network access, an API key, or local system access, so
              it can&rsquo;t run in the browser playground.
            </span>
            <span>
              See the <a href={guidesHref}>guides</a> for a recorded run, or try it locally with the{" "}
              <a href={installHref}>installed CLI</a>.
            </span>
          </div>
        )}
      </div>

      {/* Flags */}
      <div className={detailStyles.docSection}>
        <div className={detailStyles.sectionTitle}>Flags</div>
        {flags.length === 0 ? (
          <p className={detailStyles.noFlags}>This command has no local flags.</p>
        ) : (
          <table className={detailStyles.flagTable}>
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
                  <td className={detailStyles.flagName}>
                    --{f.name}
                    {f.shorthand ? `, -${f.shorthand}` : ""}
                  </td>
                  <td className={detailStyles.flagType}>{f.type ?? ""}</td>
                  <td className={detailStyles.flagDefault}>{formatDefault(f.default, f.type)}</td>
                  <td className={detailStyles.flagUsage}>{f.usage ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Additional examples */}
      {remainingExamples.length > 0 && (
        <div className={detailStyles.docSection}>
          <div className={detailStyles.sectionTitle}>More examples</div>
          <div className={detailStyles.exampleList}>
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
                <code key={i} className={detailStyles.synopsis}>
                  {ex.trim()}
                </code>
              ),
            )}
          </div>
        </div>
      )}

      <p className={styles.browseBack}>
        &larr; Back to the <a href={commandsHref}>Command Reference</a>
      </p>
    </div>
  );
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className={detailStyles.metaItem}>
      <span className={detailStyles.metaLabel}>{label}</span>
      <span className={mono ? detailStyles.metaValueMono : detailStyles.metaValue}>{value}</span>
    </div>
  );
}

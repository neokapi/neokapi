import Link from "@docusaurus/Link";
import { checks } from "@neokapi/reference-data";
import { firstLine } from "@site/src/components/reference/Markdown";
import { checkHref } from "@site/src/components/reference/slugs";
import styles from "./pages/pages.module.css";

/**
 * The index of the checks `kapi check` runs over source content, rendered from
 * the generated `@neokapi/reference-data` dataset: one row per check, in the
 * order kapi runs them, with the rule ids its findings carry. Each row links
 * to the check's static page.
 */
export default function CheckReferenceList() {
  return (
    <table className={styles.paramTable}>
      <thead>
        <tr>
          <th>Check</th>
          <th>Rule ids</th>
          <th>What it checks</th>
        </tr>
      </thead>
      <tbody>
        {checks.entries.map((entry) => (
          <tr key={entry.id}>
            <td className={styles.paramName}>
              <Link to={checkHref(entry)}>{entry.displayName}</Link>
            </td>
            <td className={styles.paramType}>
              {(entry.rules ?? []).map((rule) => (
                <div key={rule.id}>
                  <code>{rule.id}</code>
                </div>
              ))}
            </td>
            <td className={styles.paramDesc}>
              {entry.description || firstLine(entry.doc?.overview)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

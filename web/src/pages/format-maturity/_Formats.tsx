import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { formats } from "@neokapi/reference-data";
import snapshot from "../../../static/data/format-maturity.json";
import {
  AXIS_DIMS,
  AXIS_IDS,
  AXIS_LABEL,
  GRADE_NAME,
  PRESENCE_LABEL,
  PROSE_DIM_RUNG,
  TIER_DECAY_DAYS,
  TIER_LABEL,
  TIER_ORDER,
  TIER_STALE_DAYS,
  type AxisId,
  type DimScore,
  type FormatRow,
  type Grade,
  type LanguageRow,
  type MaturityData,
  type SupportTier,
  type TierInfo,
} from "./_types";
import styles from "./_Formats.module.css";

const data = snapshot as unknown as MaturityData;
const registered = new Map(formats.entries.map((entry) => [entry.id, entry]));
const rows = data.formats
  .map((row) => {
    const entry = registered.get(row.id);
    const name = entry?.displayName || row.id;
    return {
      row,
      name,
      searchable: [name, row.id, ...(entry?.extensions ?? [])].join(" ").toLowerCase(),
    };
  })
  .sort((a, b) => a.name.localeCompare(b.name, "en") || a.row.id.localeCompare(b.row.id, "en"));

const axes = AXIS_IDS.filter(
  (axis) =>
    axis === "engine" ||
    data.axes?.[axis] ||
    data.summary.by_axis?.[axis] ||
    rows.some(({ row }) => row.levels?.[axis] || row.axes?.[axis]),
);
const hasTiers = rows.some(({ row }) => row.tier);
const scoreLabel: Record<DimScore, string> = {
  complete: t("Complete", "maturity evidence"),
  partial: t("Partial", "maturity evidence"),
  none: t("Missing", "maturity evidence"),
  na: t("Not applicable", "maturity evidence"),
  notrun: t("Did not run", "maturity evidence"),
};
const unrecorded = t("Not recorded", "missing maturity observation");

type Assessment = NonNullable<NonNullable<FormatRow["axes"]>[AxisId]> & {
  floor?: string;
  ceiling?: string;
};

function grade(row: FormatRow, axis: AxisId): Grade | undefined {
  return axis === "engine" ? row.level : row.levels?.[axis];
}

function gradeName(row: FormatRow, value: Grade): string {
  if (value === "L1" && row.type === "read-only")
    return t("Readable", "read-only format maturity level");
  return GRADE_NAME[value];
}

function Certification({ tier }: { tier: TierInfo }) {
  const days = tier.last_certified
    ? Math.floor((Date.parse(data.generated_at) - Date.parse(tier.last_certified)) / 86_400_000)
    : NaN;
  const effective =
    TIER_ORDER[Math.min(TIER_ORDER.indexOf(tier.declared) + 1, TIER_ORDER.length - 1)];
  return (
    <section>
      <h3>{t("Certification", "format details")}</h3>
      <dl className={styles.facts}>
        <dt>{t("Declared support", "format details")}</dt>
        <dd>{TIER_LABEL[tier.declared]}</dd>
        {tier.since && (
          <>
            <dt>{t("Declared since", "format details")}</dt>
            <dd>{tier.since}</dd>
          </>
        )}
        <dt>{t("Last certified", "format details")}</dt>
        <dd>{tier.last_certified || t("Never certified", "format details")}</dd>
        {days > TIER_STALE_DAYS && (
          <>
            <dt>{t("Certification status", "format details")}</dt>
            <dd>
              {days > TIER_DECAY_DAYS
                ? t(
                    "Certification expired. The effective tier is {tier}.",
                    "format certification",
                    {
                      tier: TIER_LABEL[effective],
                    },
                  )
                : t("Stale at this snapshot", "format certification")}
            </dd>
          </>
        )}
        {!!tier.gates?.length && (
          <>
            <dt>{t("Certification gates", "format details")}</dt>
            <dd>
              {tier.gates.map((gate) => (
                <code className={styles.gate} key={gate}>
                  {gate}
                </code>
              ))}
            </dd>
          </>
        )}
      </dl>
    </section>
  );
}

function AxisEvidence({
  row,
  axis,
  selected,
}: {
  row: FormatRow;
  axis: AxisId;
  selected: boolean;
}) {
  const value = grade(row, axis);
  const assessment: Assessment | undefined = row.axes?.[axis];
  const next = row.next?.[axis] ?? (axis === "engine" ? row.next_level : undefined);
  const floor = assessment?.floor ?? (axis === "engine" ? row.floor : undefined);
  const ceiling = assessment?.ceiling ?? (axis === "engine" ? row.ceiling : undefined);
  const gaps = assessment?.blocking_gaps ?? (axis === "engine" ? row.blocking_gaps : []);
  const flatDims = Object.keys(row.dimensions).filter((id) =>
    data.dimension_axes
      ? data.dimension_axes[id] === axis
      : !row.levels
        ? axis === "engine"
        : AXIS_DIMS[axis].includes(id),
  );
  const dims = Array.from(new Set([...flatDims, ...Object.keys(row.dims?.[axis] ?? {})]));
  if (!value && !dims.length && !gaps?.length) return null;
  return (
    <details open={selected} className={styles.axis}>
      <summary>
        <span>{AXIS_LABEL[axis]}</span>
        <span className={styles.grade}>
          {value ? `${gradeName(row, value)} (${value})` : unrecorded}
        </span>
      </summary>
      <div className={styles.axisBody}>
        {floor && ceiling && floor !== ceiling && (
          <p>
            {t("Evidence supports {floor} through {ceiling}.", "maturity evidence", {
              floor,
              ceiling,
            })}
          </p>
        )}
        {row.last_review?.levels?.[axis] && (
          <p>
            {t("The reviewer recorded {grade}.", "format assessment", {
              grade: row.last_review.levels[axis],
            })}
          </p>
        )}
        {dims.length > 0 && (
          <dl className={styles.dimensions}>
            {dims.map((id) => {
              const score = row.dims?.[axis]?.[id] ?? row.dimensions[id];
              const rung = axis === "prose" ? assessment?.rungs?.[PROSE_DIM_RUNG[id]] : undefined;
              return (
                <div key={id}>
                  <dt>{data.dimension_labels[id] ?? id}</dt>
                  <dd>
                    {scoreLabel[score] ?? unrecorded}
                    {rung && <small>{rung.reason}</small>}
                  </dd>
                </div>
              );
            })}
          </dl>
        )}
        {!!gaps?.length && (
          <>
            <h4>
              {next
                ? t("Gaps for {next}", "format evidence", { next })
                : t("Blocking gaps", "format evidence")}
            </h4>
            <ul>
              {gaps.map((gap, i) => (
                <li key={i}>{gap}</li>
              ))}
            </ul>
          </>
        )}
        {!!assessment?.languages?.length && (
          <p>
            {t("Scored separately for {languages}.", "format evidence", {
              languages: assessment.languages.join(", "),
            })}
          </p>
        )}
      </div>
    </details>
  );
}

function FormatDetails({ row, selected }: { row: FormatRow; selected: AxisId }) {
  return (
    <div className={styles.detail}>
      <div>
        {row.tier && <Certification tier={row.tier} />}
        <h3>{t("Format record", "format details")}</h3>
        <dl className={styles.facts}>
          <dt>{t("Format ID", "format details")}</dt>
          <dd>
            <code>{row.id}</code>
          </dd>
          {row.okapi_counterpart && (
            <>
              <dt>{t("Okapi counterpart", "format details")}</dt>
              <dd>{row.okapi_counterpart}</dd>
            </>
          )}
          {row.last_review && (
            <>
              <dt>{t("Last review", "format details")}</dt>
              <dd>{row.last_review.date}</dd>
              <dt>{t("Review confidence", "format details")}</dt>
              <dd>{row.last_review.confidence}</dd>
            </>
          )}
          {row.top_risk && (
            <>
              <dt>{t("Recorded risk", "format details")}</dt>
              <dd>{row.top_risk}</dd>
            </>
          )}
        </dl>
        {row.evidence && Object.keys(row.evidence).length > 0 && (
          <>
            <h3>{t("Evidence references", "format details")}</h3>
            <dl className={styles.facts}>
              {Object.entries(row.evidence).map(([key, ref]) => (
                <div key={key}>
                  <dt>{data.dimension_labels[key] ?? key}</dt>
                  <dd>{ref}</dd>
                </div>
              ))}
            </dl>
          </>
        )}
      </div>
      <section>
        <h3>{t("Assessment by axis", "format details")}</h3>
        {axes.map((axis) => (
          <AxisEvidence key={axis} row={row} axis={axis} selected={selected === axis} />
        ))}
      </section>
    </div>
  );
}

function LanguageEvidence({ languages }: { languages: LanguageRow[] }) {
  return (
    <details className={styles.languages}>
      <summary>
        {t("Source languages ({total})", "format maturity", { total: languages.length })}
      </summary>
      {languages.map((lang) => (
        <details key={lang.id} className={styles.axis}>
          <summary>
            <span>{lang.name}</span>
            <span className={styles.grade}>
              {lang.level ? GRADE_NAME[lang.level] : PRESENCE_LABEL[lang.presence]}
            </span>
          </summary>
          <div className={styles.axisBody}>
            {lang.provider && (
              <p>{t("Read by {provider}.", "language maturity", { provider: lang.provider })}</p>
            )}
            {lang.presence_reason && <p>{lang.presence_reason}</p>}
            <dl className={styles.dimensions}>
              {Object.entries(lang.dims).map(([id, score]) => (
                <div key={id}>
                  <dt>{data.dimension_labels[id] ?? id}</dt>
                  <dd>
                    {scoreLabel[score]}
                    <small>{lang.rungs[PROSE_DIM_RUNG[id]]?.reason}</small>
                  </dd>
                </div>
              ))}
            </dl>
          </div>
        </details>
      ))}
    </details>
  );
}

export default function Formats({
  initialSearch = "",
  initialTier,
}: { initialSearch?: string; initialTier?: SupportTier } = {}) {
  const [search, setSearch] = useState(initialSearch);
  const [tier, setTier] = useState<SupportTier | "">(initialTier ?? "");
  const [axis, setAxis] = useState<AxisId>("engine");
  const [expanded, setExpanded] = useState<string | null>(null);
  const query = search.trim().toLowerCase();
  const visible = rows.filter(
    ({ row, searchable }) => (!tier || row.tier?.declared === tier) && searchable.includes(query),
  );
  return (
    <section className={styles.panel}>
      <div className={styles.heading}>
        <h2>{t("Formats", "maturity dashboard")}</h2>
        <a href="https://github.com/neokapi/neokapi/blob/main/docs/internals/format-maturity.md">
          {t("Full maturity rubric", "maturity dashboard link")}
        </a>
      </div>
      <div className={styles.filters}>
        <label>
          {t("Find a format", "format filter")}
          <input type="search" value={search} onChange={(event) => setSearch(event.target.value)} />
        </label>
        {hasTiers && (
          <label>
            {t("Support tier", "format filter")}
            <select
              value={tier}
              onChange={(event) => setTier(event.target.value as SupportTier | "")}
            >
              <option value="">{t("All tiers", "format filter")}</option>
              {TIER_ORDER.map((value) => (
                <option key={value} value={value}>
                  {TIER_LABEL[value]}
                </option>
              ))}
            </select>
          </label>
        )}
        {axes.length > 1 && (
          <label>
            {t("Assessment axis", "format filter")}
            <select value={axis} onChange={(event) => setAxis(event.target.value as AxisId)}>
              {axes.map((value) => (
                <option key={value} value={value}>
                  {AXIS_LABEL[value]}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      <p role="status" className={styles.count}>
        {t("{shown} of {total} formats", "format filter results", {
          shown: visible.length,
          total: rows.length,
        })}
      </p>
      {visible.length ? (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th scope="col">{t("Format", "format table column")}</th>
                {hasTiers && <th scope="col">{t("Declared support", "format table column")}</th>}
                <th scope="col">
                  {t("{axis} grade", "format table column", { axis: AXIS_LABEL[axis] })}
                </th>
              </tr>
            </thead>
            {visible.map(({ row, name }) => {
              const open = expanded === row.id;
              const gradeValue = grade(row, axis);
              const detailId = `format-details-${row.id}`;
              return (
                <tbody key={row.id}>
                  <tr>
                    <th scope="row">
                      <button
                        type="button"
                        aria-expanded={open}
                        aria-controls={open ? detailId : undefined}
                        onClick={() => setExpanded(open ? null : row.id)}
                      >
                        <ChevronRight size={15} aria-hidden="true" />
                        {name}
                      </button>
                    </th>
                    {hasTiers && <td>{row.tier ? TIER_LABEL[row.tier.declared] : unrecorded}</td>}
                    <td>{gradeValue ? gradeName(row, gradeValue) : unrecorded}</td>
                  </tr>
                  {open && (
                    <tr>
                      <td colSpan={hasTiers ? 3 : 2} id={detailId} className={styles.detailCell}>
                        <FormatDetails row={row} selected={axis} />
                      </td>
                    </tr>
                  )}
                </tbody>
              );
            })}
          </table>
        </div>
      ) : (
        <p>{t("No formats match these filters.", "empty format results")}</p>
      )}
      {!!data.languages?.length && <LanguageEvidence languages={data.languages} />}
    </section>
  );
}

import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import { t } from "@neokapi/i18n-react/runtime";
import { models as modelData } from "@neokapi/reference-data";
import type { ModelEntry, ModelStatus } from "@neokapi/reference-data";

// The AI model reference. kapi ships support for a curated set of models, and the
// facts about them — which provider, what ceilings, and crucially where each sits
// in its life (current, or superseded by a newer one) — used to live nowhere a
// reader could see. This page renders the committed catalog (providers/ai/models.json, via
// @neokapi/reference-data), so the list ships with the docs and cannot quietly
// describe a model kapi no longer supports: a drift gate (make check-reference-docs)
// fails the build if the page and the catalog disagree.

const cell: CSSProperties = {
  padding: "8px 12px",
  borderBottom: "1px solid var(--ifm-table-border-color)",
  textAlign: "left",
  verticalAlign: "top",
};
const num: CSSProperties = { ...cell, textAlign: "right", fontVariantNumeric: "tabular-nums" };

const statusStyle: Record<ModelStatus, CSSProperties> = {
  active: {
    background: "var(--ifm-color-success-contrast-background)",
    color: "var(--ifm-color-success-dark)",
  },
  superseded: {
    background: "var(--ifm-color-warning-contrast-background)",
    color: "var(--ifm-color-warning-dark)",
  },
};

// The status is an enum in the catalog; the pill shows the word for it.
const STATUS_LABELS: Record<ModelStatus, string> = {
  active: t("active", "model status"),
  superseded: t("superseded", "model status"),
};

function StatusPill({ status }: { status: ModelStatus }): ReactElement {
  return (
    <span
      style={{
        ...statusStyle[status],
        padding: "1px 8px",
        borderRadius: 10,
        fontSize: "0.78rem",
        fontWeight: 600,
        textTransform: "capitalize",
        whiteSpace: "nowrap",
      }}
    >
      {STATUS_LABELS[status]}
    </span>
  );
}

/** k-suffixed token count: 128000 → 128k. */
function tokens(n?: number): string {
  if (!n) return "—";
  return n >= 1000 ? `${Math.round(n / 1000)}k` : String(n);
}

function label(id: string, all: ModelEntry[]): string {
  return all.find((m) => m.id === id)?.label ?? id;
}

function ProviderTable({ rows, all }: { rows: ModelEntry[]; all: ModelEntry[] }): ReactElement {
  return (
    <div style={{ overflowX: "auto" }}>
      <table
        style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem", minWidth: 680 }}
      >
        <thead>
          <tr>
            <th style={cell}>Model</th>
            <th style={cell}>Status</th>
            <th style={cell}>In neokapi since</th>
            <th style={cell}>Lifecycle</th>
            <th style={num}>Max output</th>
            <th style={num}>Context</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((m) => (
            <tr key={m.id}>
              <td style={cell}>
                <div style={{ fontWeight: 600 }}>{m.label}</div>
                <code style={{ fontSize: "0.8rem", color: "var(--ifm-color-emphasis-600)" }}>
                  {m.id}
                </code>
                {m.aliases && m.aliases.length > 0 && (
                  <div
                    style={{
                      fontSize: "0.78rem",
                      color: "var(--ifm-color-emphasis-600)",
                      marginTop: 2,
                    }}
                  >
                    alias: {m.aliases.join(", ")}
                  </div>
                )}
                {m.defaultFor && m.defaultFor.length > 0 && (
                  <div
                    style={{ fontSize: "0.78rem", color: "var(--ifm-color-primary)", marginTop: 2 }}
                  >
                    default for {m.defaultFor.join(", ")}
                  </div>
                )}
              </td>
              <td style={cell}>
                <StatusPill status={m.status} />
              </td>
              <td style={cell}>{m.introduced || "—"}</td>
              <td style={cell}>
                {m.status === "superseded" && m.supersededBy && (
                  <span>
                    superseded by <strong>{label(m.supersededBy, all)}</strong>
                  </span>
                )}
                {m.status === "active" && (
                  <span style={{ color: "var(--ifm-color-emphasis-500)" }}>current</span>
                )}
                {m.retirementDate && (
                  <div
                    style={{
                      color: "var(--ifm-color-warning-dark)",
                      fontSize: "0.8rem",
                      marginTop: 2,
                    }}
                  >
                    retires {m.retirementDate}
                  </div>
                )}
                {m.note && (
                  <div
                    style={{
                      fontSize: "0.8rem",
                      color: "var(--ifm-color-emphasis-700)",
                      marginTop: 4,
                    }}
                  >
                    {m.note}
                  </div>
                )}
              </td>
              <td style={num}>{tokens(m.maxOutputTokens)}</td>
              <td style={num}>{tokens(m.contextWindow)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function Models(): ReactElement {
  const all = modelData.models;
  const providers = [...new Set(all.map((m) => m.provider))];

  return (
    <Layout
      title="AI Models"
      description="The models kapi ships support for — provider, output and context ceilings, and where each sits in its neokapi lifecycle (introduced, superseded, scheduled retirement)."
    >
      <main className="container margin-vert--lg" style={{ maxWidth: 960 }}>
        <h1>AI Models</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          The models kapi ships support for, with their ceilings and their place in the neokapi
          lifecycle. This is the committed catalog the binary itself reads — the same file drives
          the per-provider defaults and the output-token budgeting — so the list here is what your
          build actually supports, not a hand-maintained copy that drifts.
        </p>
        <p>
          Each provider leads with the models <strong>recommended</strong> for the work kapi does —
          translation, review, terminology — which for most projects is a short list.{" "}
          <strong>Advanced &amp; specialised</strong> holds models that are fully supported but not
          the usual choice: capable-but-premium ones (Opus, Gemini Pro) that are overkill for
          faithful content work, or ones tuned for other tasks (Fable, for creative writing).{" "}
          <strong>Legacy</strong> holds superseded models — still callable, with a newer successor.
          Nothing here is hidden or blocked: every model, in any group, can be named with{" "}
          <code>--model</code>, and a model the provider stops serving is removed from the catalog
          rather than kept as a tombstone.
        </p>
        <p>
          &ldquo;In neokapi since&rdquo; is the date the model id entered the source tree; models
          present when the catalog was first seeded share that date, and precise dates are recorded
          going forward. For what these models <em>cost</em> and how they behave under batching, see{" "}
          <a href="/batch-eval">the batch eval</a>.
        </p>

        {providers.map((p) => {
          const rows = all.filter((m) => m.provider === p);
          const providerLabel = rows[0]?.providerLabel ?? p;
          // Three buckets. Recommended active models lead; capable-but-premium or
          // off-task active models fold into "Advanced"; superseded ones into
          // "Legacy". The last two collapse, so the common case is a short list.
          const recommended = rows.filter((m) => m.status === "active" && m.recommended);
          const advanced = rows.filter((m) => m.status === "active" && !m.recommended);
          const legacy = rows.filter((m) => m.status === "superseded");
          return (
            <section key={p} style={{ marginTop: 32 }}>
              <h2 style={{ marginBottom: 8 }}>{providerLabel}</h2>
              <ProviderTable rows={recommended.length ? recommended : rows} all={all} />
              {advanced.length > 0 && (
                <details style={{ marginTop: 10 }}>
                  <summary style={{ cursor: "pointer", color: "var(--ifm-color-emphasis-700)" }}>
                    Advanced &amp; specialised ({advanced.length}) — capable but premium, or tuned
                    for other tasks
                  </summary>
                  <div style={{ marginTop: 8 }}>
                    <ProviderTable rows={advanced} all={all} />
                  </div>
                </details>
              )}
              {legacy.length > 0 && (
                <details style={{ marginTop: 10 }}>
                  <summary style={{ cursor: "pointer", color: "var(--ifm-color-emphasis-700)" }}>
                    Legacy ({legacy.length}) — superseded, still callable
                  </summary>
                  <div style={{ marginTop: 8 }}>
                    <ProviderTable rows={legacy} all={all} />
                  </div>
                </details>
              )}
            </section>
          );
        })}

        <p style={{ marginTop: 32, fontSize: "0.85rem", color: "var(--ifm-color-emphasis-600)" }}>
          Catalog reviewed {modelData.asOf}. Maintained in <code>providers/ai/models.json</code>;{" "}
          <code>make check-models</code> flags when a provider&rsquo;s live model list diverges from
          it.
        </p>
      </main>
    </Layout>
  );
}

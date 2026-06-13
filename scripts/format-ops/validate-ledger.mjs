#!/usr/bin/env node
// validate-ledger.mjs — schema-check the format-ops ledger.
//
// Contract: docs/internals/format-ops.md §4 (ledger schema) and
// format-maturity.md §3 (mutation-check evidence shape).
//
// Usage: validate-ledger.mjs [path] [--root <repo-root>]
//   path defaults to <root>/docs/internals/format-ops-ledger.json
//
// Checks:
//   - ledger_version == 1
//   - rituals: exactly the 12 known ids, no extras, none missing
//   - per-ritual required watermark families (per the §4 seed)
//   - cadence_days: integer >= 0 (0 = watermark-only); ci_owned ⇒ cadence 0
//   - last_run: null or ISO date
//   - pending[]: {id, ritual, type ∈ enum, proposal, evidence, created, expires?}
//   - runs[]: {date, ritual, commit, model_id, outcome, evidence: [{check, exit, output_sha}]}
//   - remediate.carryover[]: {format, axis, gap, attempt_date, outcome ∈ enum, evidence}
//   - process-health.adjudicated: {rubric_sha, grades}
//   - file is canonical 2-space-indented JSON (+ trailing newline)
//
// Exit 1 with precise messages on any violation.

import fs from "node:fs";
import path from "node:path";
import { DEFAULT_ROOT, parseArgs, isISODate, isPlainObject, Problems } from "./lib.mjs";

const { opts, positional } = parseArgs(process.argv.slice(2), ["--root"], []);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;
const ledgerPath = positional[0]
  ? path.resolve(positional[0])
  : path.join(root, "docs", "internals", "format-ops-ledger.json");

const p = new Problems(`validate-ledger ${path.relative(process.cwd(), ledgerPath)}`);

// The 12 rituals and their required structured fields (ops §4 seed).
const RITUALS = {
  "triage-score": {
    watermarks: ["core_formats_sha", "scorer_version", "audit_sha", "model_id", "prompt_sha", "axes_published"],
  },
  remediate: {
    watermarks: ["dashboard_generated_at"],
    arrays: ["carryover"],
    numbers: ["max_fixes_per_run"],
  },
  "parity-publish": { watermarks: ["report_generated_at", "main_sha"] },
  "contract-audit": { watermarks: ["generatedAt", "okapiTag"] },
  "upstream-watch": {
    watermarks: [
      "okapi_last_issue_iid",
      "okapi_last_issue_updated_at",
      "okapi_latest_tag",
      "okapi_pinned",
      "per_spec",
      "per_implementation",
    ],
    objects: ["per_format_last_swept"],
  },
  "xfail-hygiene": { watermarks: ["tracked_issues"] },
  "corpus-census": {
    watermarks: ["manifest_shas", "release_tag"],
    objects: ["external_verification"],
  },
  "corpus-sweep": { watermarks: ["per_format_counts"] },
  "case-gen": { watermarks: ["per_section_coverage"] },
  "format-radar": { watermarks: null, objects: ["decided"] },
  "process-health": {
    watermarks: ["model_id", "prompt_sha", "rubric_sha", "learnings_sha"],
    arrays: ["golden_set"],
    objects: ["adjudicated"],
  },
  "tier-review": { watermarks: ["support_sha"] },
};

const PENDING_TYPES = ["tier-change", "demotion", "rubric-edit", "radar-decision", "adjudication", "na-countersign"];
const CARRYOVER_OUTCOMES = ["test_failed", "landed", "skipped", "blocked"];

if (!fs.existsSync(ledgerPath)) {
  p.error(`ledger file not found: ${ledgerPath}`);
  process.exit(p.report());
}

const raw = fs.readFileSync(ledgerPath, "utf8");
let ledger;
try {
  ledger = JSON.parse(raw);
} catch (e) {
  p.error(`not valid JSON: ${e.message}`);
  process.exit(p.report());
}

// ── Canonical 2-space formatting ────────────────────────────────────────────
const canonical = JSON.stringify(ledger, null, 2) + "\n";
if (raw !== canonical) {
  const a = raw.split("\n");
  const b = canonical.split("\n");
  let line = 0;
  while (line < Math.max(a.length, b.length) && a[line] === b[line]) line++;
  p.error(
    `file is not canonical 2-space-indented JSON (first difference at line ${line + 1}: ` +
      `got ${JSON.stringify(a[line] ?? "<EOF>")}, want ${JSON.stringify(b[line] ?? "<EOF>")})`,
  );
}

// ── Top level ───────────────────────────────────────────────────────────────
if (ledger.ledger_version !== 1) {
  p.error(`ledger_version must be 1, got ${JSON.stringify(ledger.ledger_version)}`);
}
if (!isPlainObject(ledger.rituals)) {
  p.error(`"rituals" must be an object`);
}
if (!Array.isArray(ledger.pending)) p.error(`"pending" must be an array`);
if (!Array.isArray(ledger.runs)) p.error(`"runs" must be an array`);

// ── Rituals ─────────────────────────────────────────────────────────────────
const ritualIds = isPlainObject(ledger.rituals) ? Object.keys(ledger.rituals) : [];
for (const id of ritualIds) {
  if (!RITUALS[id]) p.error(`rituals: unknown ritual id "${id}" (the 12 known ids are: ${Object.keys(RITUALS).join(", ")})`);
}
for (const id of Object.keys(RITUALS)) {
  if (!ritualIds.includes(id)) p.error(`rituals: missing required ritual "${id}"`);
}

for (const [id, spec] of Object.entries(RITUALS)) {
  const r = ledger.rituals?.[id];
  if (!isPlainObject(r)) continue; // missing already reported
  const at = `rituals.${id}`;

  // cadence semantics
  if (!Number.isInteger(r.cadence_days) || r.cadence_days < 0) {
    p.error(`${at}.cadence_days must be an integer >= 0, got ${JSON.stringify(r.cadence_days)}`);
  }
  if ("ci_owned" in r && typeof r.ci_owned !== "boolean") {
    p.error(`${at}.ci_owned must be a boolean, got ${JSON.stringify(r.ci_owned)}`);
  }
  if (r.ci_owned === true && r.cadence_days !== 0) {
    p.error(`${at}: ci_owned rituals are watch-only and must have cadence_days 0, got ${r.cadence_days}`);
  }

  // last_run
  if (!(r.last_run === null || isISODate(r.last_run))) {
    p.error(`${at}.last_run must be null or an ISO date, got ${JSON.stringify(r.last_run)}`);
  }

  // blocked_on, when present, is a non-empty string (issue URL)
  if ("blocked_on" in r && (typeof r.blocked_on !== "string" || r.blocked_on === "")) {
    p.error(`${at}.blocked_on must be a non-empty string (issue URL)`);
  }

  // watermark family
  if (spec.watermarks) {
    if (!isPlainObject(r.watermarks)) {
      p.error(`${at}.watermarks must be an object with keys: ${spec.watermarks.join(", ")}`);
    } else {
      for (const key of spec.watermarks) {
        if (!(key in r.watermarks)) p.error(`${at}.watermarks: missing required watermark "${key}"`);
      }
    }
  }
  for (const key of spec.arrays ?? []) {
    if (!Array.isArray(r[key])) p.error(`${at}.${key} must be an array`);
  }
  for (const key of spec.objects ?? []) {
    if (!isPlainObject(r[key])) p.error(`${at}.${key} must be an object`);
  }
  for (const key of spec.numbers ?? []) {
    if (typeof r[key] !== "number") p.error(`${at}.${key} must be a number`);
  }
}

// format-radar.decided: {accepted: [], rejected: {}}
{
  const decided = ledger.rituals?.["format-radar"]?.decided;
  if (isPlainObject(decided)) {
    if (!Array.isArray(decided.accepted)) p.error(`rituals.format-radar.decided.accepted must be an array`);
    if (!isPlainObject(decided.rejected)) p.error(`rituals.format-radar.decided.rejected must be an object`);
  }
}

// remediate.carryover[] shape
{
  const carryover = ledger.rituals?.remediate?.carryover;
  if (Array.isArray(carryover)) {
    carryover.forEach((c, i) => {
      const at = `rituals.remediate.carryover[${i}]`;
      if (!isPlainObject(c)) return p.error(`${at} must be an object`);
      for (const f of ["format", "axis", "gap", "attempt_date", "outcome", "evidence"]) {
        if (!(f in c)) p.error(`${at}: missing required field "${f}"`);
      }
      if ("attempt_date" in c && !isISODate(c.attempt_date)) {
        p.error(`${at}.attempt_date must be an ISO date, got ${JSON.stringify(c.attempt_date)}`);
      }
      if ("outcome" in c && !CARRYOVER_OUTCOMES.includes(c.outcome)) {
        p.error(`${at}.outcome must be one of ${CARRYOVER_OUTCOMES.join("|")}, got ${JSON.stringify(c.outcome)}`);
      }
    });
  }
}

// process-health.adjudicated: {rubric_sha, grades: {<format>: {<axis>: <grade>}}}
{
  const ph = ledger.rituals?.["process-health"];
  if (isPlainObject(ph?.adjudicated)) {
    const a = ph.adjudicated;
    if (typeof a.rubric_sha !== "string") {
      p.error(`rituals.process-health.adjudicated.rubric_sha must be a string`);
    }
    if (!isPlainObject(a.grades)) {
      p.error(`rituals.process-health.adjudicated.grades must be an object ({<format>: {<axis>: <grade>}})`);
    } else {
      for (const [fmt, grades] of Object.entries(a.grades)) {
        if (!isPlainObject(grades)) {
          p.error(`rituals.process-health.adjudicated.grades.${fmt} must be an object of axis -> grade`);
        }
      }
    }
  }
  if (isPlainObject(ph) && "golden_set" in ph && Array.isArray(ph.golden_set)) {
    ph.golden_set.forEach((g, i) => {
      if (typeof g !== "string" || g === "") {
        p.error(`rituals.process-health.golden_set[${i}] must be a non-empty format id string`);
      }
    });
  }
}

// ── pending[] ───────────────────────────────────────────────────────────────
if (Array.isArray(ledger.pending)) {
  ledger.pending.forEach((item, i) => {
    const at = `pending[${i}]`;
    if (!isPlainObject(item)) return p.error(`${at} must be an object`);
    for (const f of ["id", "ritual", "type", "proposal", "evidence", "created"]) {
      if (!(f in item)) p.error(`${at}: missing required field "${f}"`);
    }
    if ("ritual" in item && !RITUALS[item.ritual]) {
      p.error(`${at}.ritual "${item.ritual}" is not a known ritual id`);
    }
    if ("type" in item && !PENDING_TYPES.includes(item.type)) {
      p.error(`${at}.type must be one of ${PENDING_TYPES.join("|")}, got ${JSON.stringify(item.type)}`);
    }
    if ("created" in item && !isISODate(item.created)) {
      p.error(`${at}.created must be an ISO date, got ${JSON.stringify(item.created)}`);
    }
    if ("expires" in item && !isISODate(item.expires)) {
      p.error(`${at}.expires must be an ISO date, got ${JSON.stringify(item.expires)}`);
    }
  });
}

// ── runs[] ──────────────────────────────────────────────────────────────────
if (Array.isArray(ledger.runs)) {
  ledger.runs.forEach((run, i) => {
    const at = `runs[${i}]`;
    if (!isPlainObject(run)) return p.error(`${at} must be an object`);
    for (const f of ["date", "ritual", "commit", "model_id", "outcome", "evidence"]) {
      if (!(f in run)) p.error(`${at}: missing required field "${f}"`);
    }
    if ("date" in run && !isISODate(run.date)) {
      p.error(`${at}.date must be an ISO date, got ${JSON.stringify(run.date)}`);
    }
    if ("ritual" in run && !RITUALS[run.ritual]) {
      p.error(`${at}.ritual "${run.ritual}" is not a known ritual id`);
    }
    // commit is a 7-40 char hex sha, or "" / "pending" for a run recorded
    // before its commit exists (e.g. a deterministic publish backfilled by the
    // committing step). A future run uses it for `git diff <commit>..HEAD`.
    if ("commit" in run && run.commit !== "" && run.commit !== "pending"
        && !/^[0-9a-f]{7,40}$/.test(String(run.commit))) {
      p.error(`${at}.commit must be a hex sha, "" or "pending", got ${JSON.stringify(run.commit)}`);
    }
    if ("model_id" in run && (typeof run.model_id !== "string" || run.model_id === "")) {
      p.error(`${at}.model_id must be a non-empty string`);
    }
    if ("outcome" in run && (typeof run.outcome !== "string" || run.outcome === "")) {
      p.error(`${at}.outcome must be a non-empty string`);
    }
    if ("evidence" in run) {
      if (!Array.isArray(run.evidence)) {
        p.error(`${at}.evidence must be an array of {check, exit, output_sha}`);
      } else {
        run.evidence.forEach((ev, j) => {
          const eat = `${at}.evidence[${j}]`;
          if (!isPlainObject(ev)) return p.error(`${eat} must be an object {check, exit, output_sha}`);
          if (typeof ev.check !== "string" || ev.check === "") p.error(`${eat}.check must be a non-empty string`);
          if (!Number.isInteger(ev.exit)) p.error(`${eat}.exit must be an integer exit status`);
          if (typeof ev.output_sha !== "string" || ev.output_sha === "") {
            p.error(`${eat}.output_sha must be a non-empty string`);
          }
        });
      }
    }
    if ("followups" in run && !Array.isArray(run.followups)) {
      p.error(`${at}.followups must be an array`);
    }
    if ("duration_min" in run && typeof run.duration_min !== "number") {
      p.error(`${at}.duration_min must be a number`);
    }
  });
}

process.exit(
  p.report(
    `${Object.keys(RITUALS).length} rituals, ${ledger.pending?.length ?? 0} pending, ${ledger.runs?.length ?? 0} runs`,
  ),
);

// The port the Bowrain web app draws the project screens through: the Map,
// the Reader and Decide read from the server's REST routes for one project
// stream, with the actions the signed-in member holds the permission for.

import { useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  useApi,
  useCallerPermissions,
  useUserDisplayNames,
  type ApiAdapter,
  type PermissionName,
  type ReaderFixResult,
} from "@neokapi/ui";
import type {
  ContextActor,
  ProjectScreensPort,
  ReaderFile,
  ReaderFixFiled,
  ScreenTarget,
} from "@neokapi/project-screens";

/** The project stream a port reads. */
export interface ScreensScope {
  ws: string;
  projectId: string;
  stream: string;
}

type ScreenRead = "map" | "reader" | "decide";

/** A port that also tells its screens to read again. */
export interface RestScreensPort extends ProjectScreensPort {
  /** Report a change to what the screens read. */
  notify(what: ScreenRead): void;
  /** Stop following a check's job; the check started is left to run on the server. */
  dispose(): void;
  /** Follow jobs again after dispose, as a remount (React StrictMode) does. */
  resume(): void;
}

export interface RestScreensOptions {
  /** The name a person is shown by. */
  actorName?: (actor: ContextActor) => string | undefined;
  /** How the check follows its job. */
  poll?: Partial<JobPolling>;
}

/** How the project-wide check follows its job. */
export interface JobPolling {
  /** The wait before the second read of the job. */
  firstMs: number;
  /** The longest wait between reads: each wait doubles up to it. */
  maxMs: number;
  /** How long the check follows a job before it gives up. */
  limitMs: number;
}

const JOB_POLLING: JobPolling = { firstMs: 1000, maxMs: 5000, limitMs: 10 * 60 * 1000 };

/**
 * The Map's file rows name each file by its path in the project, and the
 * server's Reader takes an item's name. They differ for an item stored under
 * a source path of its own, so the port reads the Map's rows to resolve one.
 */
function itemNames(api: ApiAdapter, scope: ScreensScope) {
  let rows: Map<string, string> | null = null;
  const learn = (summary: { file_rows: { path: string; relative: string }[] }) => {
    rows = new Map(summary.file_rows.map((r) => [r.relative, r.path]));
  };
  const resolve = async (path: string): Promise<string> => {
    if (!rows?.has(path)) {
      learn(await api.getMapSummary(scope.ws, scope.projectId, scope.stream));
    }
    return rows?.get(path) ?? path;
  };
  return { learn, resolve };
}

/**
 * The proposals a whole-file fix filed, set on the findings each one fixes.
 * The server files one proposal per block, in the file's block order, for
 * every block with a fixable finding none of whose fixes it refused.
 */
export function proposalsByFinding(
  before: ReaderFile,
  result: ReaderFixResult,
): ReaderFixFiled["proposed"] {
  const refused = new Set(result.refused.map((r) => r.finding_id));
  const out: ReaderFixFiled["proposed"] = [];
  let next = 0;
  for (const block of before.blocks) {
    const fixable = block.findings.filter((f) => f.fixable);
    if (fixable.length === 0 || fixable.some((f) => refused.has(f.id))) continue;
    const proposal = result.proposals[next++];
    if (!proposal) break;
    for (const f of fixable) out.push({ finding_id: f.id, proposal });
  }
  return out;
}

/** The project screens' port over the REST adapter, with every action. */
export function restScreensPort(
  api: ApiAdapter,
  scope: ScreensScope,
  opts: RestScreensOptions = {},
): RestScreensPort {
  const { ws, projectId, stream } = scope;
  const listeners = new Set<(what: ScreenRead) => void>();
  const names = itemNames(api, scope);
  const poll = { ...JOB_POLLING, ...opts.poll };
  const notify = (what: ScreenRead) => {
    for (const l of listeners) l(what);
  };
  // The waits of the jobs being followed, so dispose can end them.
  let disposed = false;
  const waits = new Set<() => void>();
  const wait = (ms: number) => {
    if (disposed) return Promise.resolve();
    return new Promise<void>((resolve) => {
      const done = () => {
        clearTimeout(timer);
        waits.delete(done);
        resolve();
      };
      const timer = setTimeout(done, ms);
      waits.add(done);
    });
  };
  return {
    notify,
    resume() {
      disposed = false;
    },
    dispose() {
      disposed = true;
      for (const done of waits) done();
      listeners.clear();
    },
    async mapSummary() {
      const summary = await api.getMapSummary(ws, projectId, stream);
      names.learn(summary);
      return summary;
    },
    async readerFile(path) {
      return api.getReaderFile(ws, projectId, await names.resolve(path), stream);
    },
    decideView: () => api.getDecideView(ws, projectId, stream),
    keptRules: () => api.getKeptRules(ws, projectId, stream),
    decideConsequence: (id, form) => api.getDecideConsequence(ws, projectId, id, form, stream),
    async decide(id, answer, form) {
      const out = await api.decideRule(ws, projectId, { id, answer, form }, stream);
      notify("map");
      return out;
    },
    async assign(id, userId) {
      await api.assignSuggestion(ws, projectId, id, userId, stream);
      notify("decide");
    },
    async fixFinding(path, findingId) {
      const res = await api.fixFinding(ws, projectId, await names.resolve(path), findingId, stream);
      return {
        proposed: res.proposals
          .slice(0, 1)
          .map((proposal) => ({ finding_id: findingId, proposal })),
        refused: res.refused,
      };
    },
    async fixFile(path) {
      const item = await names.resolve(path);
      const before = await api.getReaderFile(ws, projectId, item, stream);
      const res = await api.fixFile(ws, projectId, item, stream);
      return { proposed: proposalsByFinding(before, res), refused: res.refused };
    },
    async runChecks() {
      const { job_id } = await api.runProjectChecks(ws, projectId, stream);
      const giveUpAt = Date.now() + poll.limitMs;
      let delay = poll.firstMs;
      for (;;) {
        if (disposed) return;
        const job = await api.getJob(ws, job_id);
        if (job.status === "completed") break;
        if (job.status === "failed") throw new Error(job.error || t("The check did not finish"));
        if (Date.now() >= giveUpAt) throw new Error(t("The check did not finish"));
        await wait(Math.min(delay, Math.max(0, giveUpAt - Date.now())));
        delay = Math.min(delay * 2, poll.maxMs);
      }
      if (!disposed) notify("map");
    },
    actorName: (actor) => opts.actorName?.(actor),
    subscribe(onChange) {
      listeners.add(onChange);
      return () => listeners.delete(onChange);
    },
  };
}

/**
 * The permissions a member needs to answer a question: any that governs a
 * rule. The server checks the one the question's subject takes, so a member
 * holding another sees Keep and the server's refusal in the question's pane.
 */
const GOVERNING: PermissionName[] = ["manage_terms", "manage_voice", "manage_memory"];

/**
 * The port with only the actions the member may take: Decide's answers and
 * assignments with a permission that governs rules, and Fix (which files a
 * source proposal) and the project-wide check with review, as the server's
 * routes require.
 */
export function permittedPort<P extends ProjectScreensPort>(
  port: P,
  can: (permission: PermissionName) => boolean,
): ProjectScreensPort {
  const out: ProjectScreensPort = { ...port };
  if (!GOVERNING.some((p) => can(p))) {
    delete out.decide;
    delete out.decideConsequence;
    delete out.assign;
  }
  if (!can("review")) {
    delete out.fixFinding;
    delete out.fixFile;
  }
  if (!can("review")) delete out.runChecks;
  return out;
}

/** The targets the web app opens: the Map, the Reader, Rules and the Source content page. */
export function bowrainOpens(target: ScreenTarget): boolean {
  switch (target.kind) {
    case "map":
    case "reader":
    case "rules":
    case "kept":
    case "blocks":
      return true;
    default:
      return false;
  }
}

/**
 * The port for one project stream, the same object while the stream and the
 * member's permissions are. Its screens read again when the workspace's
 * change stream reports a change to the project: the stream invalidates the
 * project's queries, this one among them.
 */
export function useProjectScreensPort(
  scope: ScreensScope,
  me: { name?: string } | null,
): ProjectScreensPort {
  const api = useApi();
  const { ws, projectId, stream } = scope;
  const { nameOf } = useUserDisplayNames();
  const { can, data: permissions } = useCallerPermissions(projectId);

  // The names a person is shown by: the server leaves the reader's own name
  // out, and names another member by their id when it holds no name for them.
  const names = useRef({ nameOf, me });
  names.current = { nameOf, me };

  const rest = useMemo(
    () =>
      restScreensPort(
        api,
        { ws, projectId, stream },
        {
          actorName: (actor) => {
            if (actor.kind !== "person") return undefined;
            if (!actor.name) return names.current.me?.name || undefined;
            return names.current.nameOf(actor.name);
          },
        },
      ),
    [api, ws, projectId, stream],
  );
  // eslint-disable-next-line react-hooks/exhaustive-deps -- `can` reads `permissions`.
  const port = useMemo(() => permittedPort(rest, can), [rest, permissions]);

  const { dataUpdatedAt } = useQuery({
    queryKey: ["project", ws, projectId, stream, "screens"],
    queryFn: async () => Date.now(),
    staleTime: Infinity,
  });
  useEffect(() => {
    rest.resume();
    return () => rest.dispose();
  }, [rest]);

  const seen = useRef(dataUpdatedAt);
  useEffect(() => {
    if (seen.current === dataUpdatedAt) return;
    seen.current = dataUpdatedAt;
    rest.notify("map");
    rest.notify("reader");
    rest.notify("decide");
  }, [rest, dataUpdatedAt]);

  return port;
}

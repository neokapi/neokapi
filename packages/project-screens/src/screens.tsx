// The three screens read through a ProjectScreensPort: each reads what it
// draws from the port, reads again when the port reports a change, and offers
// the actions the port implements. A host with a data layer of its own can
// draw the views directly instead.

import { useCallback, useEffect, useRef, useState } from "react";
import { ErrorNotice, LoadingSpinner } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { DecideView, answerArgs } from "./DecideView";
import { MapView, type MapViewProps } from "./MapView";
import { ReaderView } from "./ReaderView";
import type { ProjectScreensPort, ScreenNavigation } from "./port";
import type { ReaderFile, ReaderRefusal } from "./types";

/** How long a typed form rests before what keeping it does is read. */
const OTHER_DEBOUNCE_MS = 250;

/**
 * One read through the port: its answer, its error, and a way to read again.
 * It reads again when the port reports a change to what it reads, and when
 * `of` (what it reads, such as a path) changes.
 */
function usePortRead<T>(
  port: ProjectScreensPort,
  what: "map" | "reader" | "decide",
  read: () => Promise<T>,
  of = "",
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const readRef = useRef(read);
  readRef.current = read;
  const reload = useCallback(() => {
    readRef.current().then(
      (next) => {
        setData(next);
        setError(null);
      },
      (err: unknown) => setError(err),
    );
  }, []);
  useEffect(() => {
    reload();
    return port.subscribe?.((changed) => {
      if (changed === what) reload();
    });
  }, [port, what, reload, of]);
  return { data, setData, error, reload };
}

/** What a screen draws while its first read is in flight, or when it fails. */
function Pending({ error, title }: { error: unknown; title: string }) {
  return (
    <div className="p-6">
      {error ? <ErrorNotice error={error} title={title} /> : <LoadingSpinner />}
    </div>
  );
}

export interface MapScreenProps
  extends
    ScreenNavigation,
    Pick<
      MapViewProps,
      "deeperLook" | "connectAI" | "suggestionsAction" | "setupPending" | "addFiles"
    > {
  port: ProjectScreensPort;
  projectName: string;
}

/** The Map, read through the port. */
export function MapScreen({ port, projectName, ...rest }: MapScreenProps) {
  const { data, error, reload } = usePortRead(port, "map", () => port.mapSummary());
  const [running, setRunning] = useState(false);
  const [runError, setRunError] = useState<unknown>(null);
  if (!data) return <Pending error={error} title={t("The project could not be read")} />;
  const runChecks = port.runChecks?.bind(port);
  return (
    <MapView
      {...rest}
      projectName={projectName}
      summary={data}
      running={running}
      runError={runError}
      onRun={
        runChecks &&
        (() => {
          setRunning(true);
          setRunError(null);
          runChecks()
            .catch((err: unknown) => setRunError(err))
            .finally(() => {
              setRunning(false);
              reload();
            });
        })
      }
    />
  );
}

export interface ReaderScreenProps extends ScreenNavigation {
  port: ProjectScreensPort;
  /** The file, project-relative. */
  path: string;
  focusBlockID?: string;
  onBack?: () => void;
  backLabel?: string;
}

/** The Reader on one file, read through the port. */
export function ReaderScreen({ port, path, ...rest }: ReaderScreenProps) {
  const { data, setData, error } = usePortRead(port, "reader", () => port.readerFile(path), path);
  // A fix answers with the file read again. Its refusals are kept apart from
  // the read: a later read carries no refusal of its own.
  const [refused, setRefused] = useState<ReaderRefusal[] | null>(null);
  const [fixing, setFixing] = useState<{ id?: string; all?: boolean } | null>(null);
  const [fixError, setFixError] = useState<unknown>(null);
  if (!data) return <Pending error={error} title={t("The file could not be read")} />;
  const fix = (run: () => Promise<ReaderFile>, at: { id?: string; all?: boolean }) => {
    setFixing(at);
    setFixError(null);
    run()
      .then((next) => {
        setData(next);
        setRefused(next.refused);
      })
      .catch((err: unknown) => setFixError(err))
      .finally(() => setFixing(null));
  };
  const fixFinding = port.fixFinding?.bind(port);
  const fixFile = port.fixFile?.bind(port);
  return (
    <ReaderView
      {...rest}
      reader={data}
      refused={refused ?? data.refused}
      busy={fixing !== null}
      fixingID={fixing?.id}
      fixingAll={!!fixing?.all}
      fixError={fixError}
      onFix={fixFinding && ((id) => fix(() => fixFinding(path, id), { id }))}
      onFixAll={fixFile && (() => fix(() => fixFile(path), { all: true }))}
    />
  );
}

export interface DecideScreenProps extends ScreenNavigation {
  port: ProjectScreensPort;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  defaultSelected?: string;
  keyboard?: boolean;
}

/** Decide for one project, read through the port. */
export function DecideScreen({ port, ...rest }: DecideScreenProps) {
  const { data, error } = usePortRead(port, "decide", () => port.decideView());
  const decide = port.decide?.bind(port);
  const decideConsequence = port.decideConsequence?.bind(port);

  // What keeping a typed form does, read once the person stops typing.
  const [typed, setTyped] = useState<{ id: string; form: string } | null>(null);
  const [consequence, setConsequence] = useState<{
    id: string;
    form: string;
    text: string;
  } | null>(null);
  useEffect(() => {
    if (!decideConsequence || !typed?.id || !typed.form) return;
    let current = true;
    const handle = window.setTimeout(() => {
      void decideConsequence(typed.id, typed.form).then((text) => {
        if (current) setConsequence({ ...typed, text });
      });
    }, OTHER_DEBOUNCE_MS);
    return () => {
      current = false;
      window.clearTimeout(handle);
    };
  }, [typed, decideConsequence]);

  return (
    <DecideView
      {...rest}
      view={data}
      loading={!data && !error}
      error={error}
      nameOf={port.actorName?.bind(port)}
      onAnswer={
        decide &&
        ((d, answer, pick) => {
          const { id, form } = answerArgs(d, answer, pick);
          return decide(id, answer, form || undefined);
        })
      }
      onOtherForm={(d, form) => setTyped({ id: d.id ?? "", form: form.trim() })}
      otherConsequence={(d, form) =>
        consequence && consequence.id === d.id && consequence.form === form
          ? consequence.text
          : undefined
      }
    />
  );
}

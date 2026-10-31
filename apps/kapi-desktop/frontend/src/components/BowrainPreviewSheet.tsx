// The Bowrain preview: the project's Map and Rules as a Bowrain project page
// draws them, read-only (page-patterns "Reader"). The Map names the project,
// and a label beside the title names where the screens read it: the Bowrain
// server, or this Mac. The Map and Rules are tabs. A file on the Map
// opens the Reader in the sheet, and Back returns to the Map. The Rules tab
// holds the questions waiting for a person and Kept by, the rules in force
// with who kept each.
//
// The sheet draws the screens through the port it is given. A port with the
// reads alone draws no Keep, no Fix and no check action. A Map, Decide or
// Kept by read the server refuses replaces the body with the server's words
// and the one action that answers them; a Reader read it refuses shows the
// same in place of the file, with Back to the Map. A connected project that
// this Mac holds no sign-in for is previewed from this Mac, with a second
// label naming the server and the command that signs in.

import { useMemo, useState } from "react";
import { ArrowLeft, ExternalLink, ShieldAlert } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  DecideScreen,
  KeptRulesScreen,
  MapScreen,
  ReaderScreen,
  type ProjectScreensPort,
  type ScreenTarget,
} from "@neokapi/project-screens";
import {
  Badge,
  Button,
  EmptyState,
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { PreviewRefusalError, previewOpens, type PreviewSource } from "../lib/previewPort";
import type { PreviewRefusal } from "../types/api";

type PreviewTab = "map" | "rules";

export interface BowrainPreviewSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectName: string;
  /** What the screens read through; keep it stable across renders. */
  port: ProjectScreensPort;
  /** Where the port reads the project. */
  source?: PreviewSource;
  /** The Bowrain server the recipe connects to, which Sign in names. */
  serverURL?: string;
  /** The project on Bowrain, which Open Bowrain opens. */
  projectURL?: string | null;
  /** The recipe connects to serverURL and this Mac holds no sign-in for it. */
  signedOut?: boolean;
  /** The tab the sheet opens on. */
  defaultTab?: PreviewTab;
}

export function BowrainPreviewSheet({
  open,
  onOpenChange,
  projectName,
  port,
  source = "local",
  serverURL,
  projectURL,
  signedOut,
  defaultTab = "map",
}: BowrainPreviewSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        data-slot="bowrain-preview"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-3xl lg:max-w-5xl"
      >
        <SheetHeader className="flex-row flex-wrap items-center gap-2 border-b border-border">
          <SheetTitle data-slot="bowrain-preview-title">
            {t("Preview", "the Bowrain preview's title")}
          </SheetTitle>
          <Badge variant="outline" data-slot="bowrain-preview-source" data-source={source}>
            {source === "bowrain"
              ? t("As Bowrain shows it", "the Bowrain preview's source label")
              : t("As this Mac shows it", "the Bowrain preview's source label")}
          </Badge>
          {signedOut && serverURL && (
            <>
              <Badge variant="outline" data-slot="bowrain-preview-signed-out">
                {t("Not signed in to {server}", "a preview refusal", {
                  server: serverHost(serverURL),
                })}
              </Badge>
              <SignInCommand serverURL={serverURL} />
            </>
          )}
        </SheetHeader>
        {open && (
          <PreviewBody
            projectName={projectName}
            port={port}
            defaultTab={defaultTab}
            serverURL={serverURL}
            projectURL={projectURL}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}

/**
 * The port with every read watched for a refusal, which onRefusal receives
 * before the read rejects as it would have, and onReaderRefusal for a Reader
 * read.
 */
function watchRefusals(
  port: ProjectScreensPort,
  onRefusal: (r: PreviewRefusal) => void,
  onReaderRefusal: (r: PreviewRefusal) => void,
): ProjectScreensPort {
  const watchWith =
    (on: (r: PreviewRefusal) => void) =>
    <T,>(read: Promise<T>): Promise<T> =>
      read.catch((e: unknown) => {
        if (e instanceof PreviewRefusalError) on(e.refusal);
        throw e;
      });
  const watch = watchWith(onRefusal);
  const watchReader = watchWith(onReaderRefusal);
  return {
    mapSummary: () => watch(port.mapSummary()),
    readerFile: (path) => watchReader(port.readerFile(path)),
    decideView: () => watch(port.decideView()),
    keptRules: port.keptRules ? () => watch(port.keptRules!()) : undefined,
    actorName: port.actorName?.bind(port),
  };
}

/** The tabs and what each draws; mounted while the sheet is open, so it opens on the Map. */
function PreviewBody({
  projectName,
  port,
  defaultTab,
  serverURL,
  projectURL,
}: {
  projectName: string;
  port: ProjectScreensPort;
  defaultTab: PreviewTab;
  serverURL?: string;
  projectURL?: string | null;
}) {
  const [tab, setTab] = useState<PreviewTab>(defaultTab);
  const [reading, setReading] = useState<{ path: string; block?: string } | null>(null);
  const [row, setRow] = useState<string | undefined>(undefined);
  const [refusal, setRefusal] = useState<PreviewRefusal | null>(null);
  const [readerRefusal, setReaderRefusal] = useState<PreviewRefusal | null>(null);
  const watched = useMemo(() => watchRefusals(port, setRefusal, setReaderRefusal), [port]);

  if (refusal) {
    return <RefusalBody refusal={refusal} serverURL={serverURL} projectURL={projectURL} />;
  }

  const navigate = (target: ScreenTarget) => {
    switch (target.kind) {
      case "map":
        setReading(null);
        setTab("map");
        return;
      case "reader":
        setReaderRefusal(null);
        setReading({ path: target.path, block: target.block });
        setTab("map");
        return;
      case "rules":
        setRow(target.row);
        setTab("rules");
        return;
      case "kept":
        setTab("rules");
        return;
    }
  };
  const nav = { navigate, canNavigate: previewOpens };

  return (
    <Tabs
      value={tab}
      onValueChange={(v) => setTab(v as PreviewTab)}
      className="flex min-h-0 flex-1 flex-col gap-0"
    >
      <TabsList variant="line" className="shrink-0 px-4">
        <TabsTrigger value="map" data-slot="bowrain-preview-tab-map">
          Map
        </TabsTrigger>
        <TabsTrigger value="rules" data-slot="bowrain-preview-tab-rules">
          Rules
        </TabsTrigger>
      </TabsList>
      <TabsContent value="map" className="min-h-0 flex-1 overflow-y-auto">
        {reading && readerRefusal ? (
          <RefusalBody
            refusal={readerRefusal}
            serverURL={serverURL}
            projectURL={projectURL}
            onBack={() => {
              setReaderRefusal(null);
              setReading(null);
            }}
          />
        ) : reading ? (
          <ReaderScreen
            {...nav}
            port={watched}
            path={reading.path}
            focusBlockID={reading.block}
            onBack={() => setReading(null)}
          />
        ) : (
          <MapScreen {...nav} port={watched} projectName={projectName} />
        )}
      </TabsContent>
      <TabsContent value="rules" className="min-h-0 flex-1 overflow-y-auto">
        <div className="h-[28rem] border-b border-border">
          <DecideScreen {...nav} key={row} port={watched} defaultSelected={row} keyboard={false} />
        </div>
        <div className="p-6">
          <KeptRulesScreen port={watched} />
        </div>
      </TabsContent>
    </Tabs>
  );
}

/** The command that signs this Mac in to a server. */
export function signInCommand(serverURL?: string): string {
  return serverURL ? `kapi auth login --server ${serverURL}` : "kapi auth login";
}

/** A server URL without its scheme. */
function serverHost(serverURL: string): string {
  return serverURL.replace(/^[a-z]+:\/\//, "");
}

/** Sign in, as the command that signs this Mac in to the server. */
function SignInCommand({ serverURL }: { serverURL?: string }) {
  return (
    <span className="flex flex-wrap items-center justify-center gap-2">
      <span className="text-xs font-medium">{t("Sign in")}</span>
      <Badge
        variant="outline"
        data-slot="bowrain-preview-sign-in"
        translate="no"
        className="font-mono text-[11px] font-normal text-muted-foreground"
      >
        {signInCommand(serverURL)}
      </Badge>
    </span>
  );
}

/**
 * What the server said, word for word, with the one action that answers it:
 * Sign in for a sign-in the server does not take or this Mac does not hold,
 * Open Bowrain for a permission or a project the person settles there. In
 * the Reader, Back returns to the Map.
 */
function RefusalBody({
  refusal,
  serverURL,
  projectURL,
  onBack,
}: {
  refusal: PreviewRefusal;
  serverURL?: string;
  projectURL?: string | null;
  onBack?: () => void;
}) {
  const host = serverURL ? serverHost(serverURL) : "";
  const title =
    refusal.message ||
    (host
      ? t("Not signed in to {server}", "a preview refusal", { server: host })
      : t("Not signed in to Bowrain", "a preview refusal"));
  let action: React.ReactNode = null;
  if (refusal.action === "sign_in") {
    action = <SignInCommand serverURL={serverURL} />;
  } else if (refusal.action === "open" && projectURL) {
    action = (
      <Button size="sm" variant="outline" asChild>
        <a
          href={projectURL}
          target="_blank"
          rel="noopener noreferrer"
          data-slot="bowrain-preview-open"
        >
          Open Bowrain
          <ExternalLink size={13} aria-hidden />
        </a>
      </Button>
    );
  }
  return (
    <div data-slot="bowrain-preview-refusal" role="alert" className="p-6">
      {onBack && (
        <Button size="sm" variant="ghost" className="mb-3" onClick={onBack}>
          <ArrowLeft size={13} aria-hidden />
          Back
        </Button>
      )}
      <EmptyState
        icon={<ShieldAlert size={20} className="mx-auto" aria-hidden />}
        title={title}
        description={refusal.status ? t("HTTP {status}", { status: refusal.status }) : undefined}
        action={action}
      />
    </div>
  );
}

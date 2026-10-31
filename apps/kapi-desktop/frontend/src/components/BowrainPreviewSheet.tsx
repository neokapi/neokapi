// The Bowrain preview: the project's Map and Rules as a Bowrain project page
// draws them, read-only (page-patterns "Reader"). The title names the
// project; the Map and Rules are tabs. A file on the Map opens the Reader in
// the sheet, and Back returns to the Map. The Rules tab holds the questions
// waiting for a person and Kept by, the rules in force with who kept each.
//
// The sheet draws the screens through the port it is given. A port with the
// reads alone draws no Keep, no Fix and no check action.

import { useState } from "react";
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
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { previewOpens } from "../lib/previewPort";

type PreviewTab = "map" | "rules";

export interface BowrainPreviewSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectName: string;
  /** What the screens read through; keep it stable across renders. */
  port: ProjectScreensPort;
  /** The tab the sheet opens on. */
  defaultTab?: PreviewTab;
}

export function BowrainPreviewSheet({
  open,
  onOpenChange,
  projectName,
  port,
  defaultTab = "map",
}: BowrainPreviewSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        data-slot="bowrain-preview"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-3xl lg:max-w-5xl"
      >
        <SheetHeader className="border-b border-border">
          <SheetTitle data-slot="bowrain-preview-title">
            {t("{project} as Bowrain shows it", "the Bowrain preview's title", {
              project: projectName,
            })}
          </SheetTitle>
        </SheetHeader>
        {open && <PreviewBody projectName={projectName} port={port} defaultTab={defaultTab} />}
      </SheetContent>
    </Sheet>
  );
}

/** The tabs and what each draws; mounted while the sheet is open, so it opens on the Map. */
function PreviewBody({
  projectName,
  port,
  defaultTab,
}: {
  projectName: string;
  port: ProjectScreensPort;
  defaultTab: PreviewTab;
}) {
  const [tab, setTab] = useState<PreviewTab>(defaultTab);
  const [reading, setReading] = useState<{ path: string; block?: string } | null>(null);
  const [row, setRow] = useState<string | undefined>(undefined);

  const navigate = (target: ScreenTarget) => {
    switch (target.kind) {
      case "map":
        setReading(null);
        setTab("map");
        return;
      case "reader":
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
        {reading ? (
          <ReaderScreen
            {...nav}
            port={port}
            path={reading.path}
            focusBlockID={reading.block}
            onBack={() => setReading(null)}
          />
        ) : (
          <MapScreen {...nav} port={port} projectName={projectName} />
        )}
      </TabsContent>
      <TabsContent value="rules" className="min-h-0 flex-1 overflow-y-auto">
        <div className="h-[28rem] border-b border-border">
          <DecideScreen {...nav} key={row} port={port} defaultSelected={row} keyboard={false} />
        </div>
        <div className="p-6">
          <KeptRulesScreen port={port} />
        </div>
      </TabsContent>
    </Tabs>
  );
}

// The channel map: where a project's content sits.
//
// Each channel is a row: a coordinate chip in the channel hue, the product it
// is a surface of, how many items sit there, and the voice governing there. A
// channel a profile declares can be renamed; one that exists only because a
// collection names it is derived, and read-only. Per-channel review coverage is
// a follow-up.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Pencil, Plus, X } from "lucide-react";
import { Badge, Button, CoordinateChip, Input, SimpleTooltip, toast } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../../hooks/useApi";
import { qk } from "../../lib/queryKeys";
import type { ChannelMapResult, ChannelMapRow, KapiProject } from "../../types/api";

/** One channel row: chip, product, item count, voice, and rename or the
 * derived note. */
export function ChannelRow({
  channel,
  onRename,
}: {
  channel: ChannelMapRow;
  /** Absent for a derived channel, which is read-only. */
  onRename?: (next: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(channel.channel);

  if (editing) {
    return (
      <li className="flex flex-wrap items-center gap-2 py-2" data-testid="channel-row">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="h-7 max-w-[12rem]"
          aria-label={t("New channel name")}
          autoFocus
        />
        <Button
          size="icon-xs"
          aria-label={t("Save")}
          onClick={() => {
            const next = draft.trim();
            if (next && next !== channel.channel) onRename?.(next);
            setEditing(false);
          }}
        >
          <Check className="size-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={t("Cancel")}
          onClick={() => {
            setDraft(channel.channel);
            setEditing(false);
          }}
        >
          <X className="size-3.5" />
        </Button>
      </li>
    );
  }

  return (
    <li className="flex flex-wrap items-center gap-2 py-2" data-testid="channel-row">
      <CoordinateChip axis="channel" value={channel.channel} />
      <span className="text-[11px] text-muted-foreground">{channel.profile}</span>
      <span className="flex-1" />
      <span className="text-[11px] text-muted-foreground" data-testid="channel-items">
        {t("{count} items", { count: channel.item_count })}
      </span>
      {channel.voice ? (
        <SimpleTooltip content={t("Edit its voice on the Voice page")}>
          <Badge variant="secondary" className="font-normal" data-testid="channel-voice">
            {channel.voice}
          </Badge>
        </SimpleTooltip>
      ) : (
        <span className="text-[11px] text-muted-foreground">{t("No voice profile")}</span>
      )}
      {channel.declared ? (
        onRename && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t("Rename {channel}", { channel: channel.channel })}
            onClick={() => setEditing(true)}
          >
            <Pencil className="size-3.5" />
          </Button>
        )
      ) : (
        <SimpleTooltip content={t("This channel exists only because a collection names it")}>
          <Badge
            variant="outline"
            className="font-normal text-muted-foreground"
            data-testid="channel-derived"
          >
            {t("declared by collections")}
          </Badge>
        </SimpleTooltip>
      )}
    </li>
  );
}

/** The add-a-channel affordance: a product and a channel slug. */
function AddChannel({ onAdd }: { onAdd: (profile: string, channel: string) => void }) {
  const [profile, setProfile] = useState("");
  const [channel, setChannel] = useState("");
  const canAdd = profile.trim() !== "" && channel.trim() !== "";
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        placeholder={t("product, e.g. campaign")}
        value={profile}
        onChange={(e) => setProfile(e.target.value)}
        className="h-7 max-w-[11rem]"
        aria-label={t("Product")}
      />
      <span className="text-muted-foreground">/</span>
      <Input
        placeholder={t("channel, e.g. promo")}
        value={channel}
        onChange={(e) => setChannel(e.target.value)}
        className="h-7 max-w-[11rem]"
        aria-label={t("Channel")}
      />
      <Button
        variant="ghost"
        size="sm"
        disabled={!canAdd}
        onClick={() => {
          onAdd(profile.trim(), channel.trim());
          setProfile("");
          setChannel("");
        }}
      >
        <Plus className="mr-1 size-3" />
        {t("Add channel")}
      </Button>
    </div>
  );
}

export interface ChannelMapProps {
  tabID: string;
  /** Called with the updated recipe after a declare or rename, to keep the
   * parent's project in step with the write. */
  onUpdate?: (project: KapiProject) => void;
  /** Injected in tests and stories; production reads the Wails backend. */
  channels?: ChannelMapRow[];
}

/** The channel map for a project: the rows, and the add affordance. */
export function ChannelMap({ tabID, onUpdate, channels: injected }: ChannelMapProps) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: qk.channelMap(tabID),
    queryFn: () => call<ChannelMapResult>("ChannelMap", tabID),
    enabled: injected === undefined && !!tabID,
  });
  const rows = injected ?? query.data?.channels ?? [];
  const refresh = () => void qc.invalidateQueries({ queryKey: qk.channelMap(tabID) });

  const declare = async (profile: string, channel: string) => {
    try {
      const project = await call<KapiProject>("DeclareChannel", tabID, profile, channel);
      if (project) onUpdate?.(project);
      refresh();
    } catch (err) {
      toast.error(`Failed to add channel: ${(err as Error).message}`);
    }
  };

  const rename = async (row: ChannelMapRow, next: string) => {
    try {
      const project = await call<KapiProject>(
        "RenameChannel",
        tabID,
        row.profile,
        row.channel,
        next,
      );
      if (project) onUpdate?.(project);
      refresh();
    } catch (err) {
      toast.error(`Failed to rename channel: ${(err as Error).message}`);
    }
  };

  return (
    <div className="space-y-3" data-testid="channel-map">
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t(
            "No channels yet. A collection sits at the project's own point until one names a channel.",
          )}
        </p>
      ) : (
        <ul className="divide-y">
          {rows.map((row) => (
            <ChannelRow
              key={row.ref}
              channel={row}
              onRename={row.declared ? (next) => void rename(row, next) : undefined}
            />
          ))}
        </ul>
      )}
      <AddChannel onAdd={(p, c) => void declare(p, c)} />
      <p className="text-[10px] text-muted-foreground">
        {t(
          "A channel is a surface of a product. A collection names one to say where its content sits.",
        )}
      </p>
    </div>
  );
}

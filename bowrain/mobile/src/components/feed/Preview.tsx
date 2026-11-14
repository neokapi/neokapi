// The body of a feed card: what the change looked like, the way a post shows
// its media. One preview per kind, drawn from the entry's counts, samples,
// standing and diff. A chip that names a language opens that language's deck.
import { targetChanges, type FeedEntry } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { countLabel, languageLabel, stateLabel } from "../../lib/feed";
import { Chip, ChipRow, PRISM_FILLS, ProgressRow, SplitBar } from "./parts";

export interface PreviewProps {
  entry: FeedEntry;
  /** Opens the review deck for a language. */
  onOpenLanguage?: (locale: string) => void;
}

const CONTEXT_OPS = ["add", "edit", "keep", "drop", "retire"];

function PushPreview({ entry }: PreviewProps) {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  const c = entry.counts;
  return (
    <ChipRow>
      {entry.stream ? (
        <Chip leading={<Ionicons name="git-branch-outline" size={13} color={String(muted)} />}>
          {entry.stream}
        </Chip>
      ) : null}
      {c.collections ? <Chip>{t("feed.chip.collections", { count: c.collections })}</Chip> : null}
      {c.files !== undefined ? <Chip>{t("feed.chip.files", { count: c.files })}</Chip> : null}
      {c.blocks !== undefined ? (
        <Chip tone="accent">{t("feed.chip.blocks", { count: c.blocks })}</Chip>
      ) : null}
      {c.files_removed ? (
        <Chip tone="removed">{t("feed.chip.filesRemoved", { count: c.files_removed })}</Chip>
      ) : null}
    </ChipRow>
  );
}

function RunPreview({ entry, onOpenLanguage }: PreviewProps) {
  const { t, i18n } = useTranslation();
  const standing = entry.standing ?? [];
  if (standing.length === 0) {
    return (
      <ChipRow>
        {(entry.locales ?? []).map((l) => (
          <Chip key={l} onPress={onOpenLanguage ? () => onOpenLanguage(l) : undefined}>
            {languageLabel(l, i18n.language)}
          </Chip>
        ))}
      </ChipRow>
    );
  }
  return (
    <View className="gap-3">
      {standing.map((s, i) => {
        const units = Math.max(s.units ?? 0, s.produced ?? 0);
        const fraction = units > 0 ? (s.produced ?? 0) / units : s.state === "shippable" ? 1 : 0;
        const state = t(`feed.standing.${s.state}`, { defaultValue: s.state });
        return (
          <ProgressRow
            key={s.locale}
            label={languageLabel(s.locale, i18n.language)}
            fraction={fraction}
            value={`${t("feed.unit.produced", { count: s.produced ?? 0 })} · ${state}`}
            fill={PRISM_FILLS[i % PRISM_FILLS.length]}
            onPress={onOpenLanguage ? () => onOpenLanguage(s.locale) : undefined}
          />
        );
      })}
    </View>
  );
}

function ReviewPreview({ entry, onOpenLanguage }: PreviewProps) {
  const { t, i18n } = useTranslation();
  const c = entry.counts;
  const locale = entry.locales?.[0];
  return (
    <View className="gap-3">
      {locale ? (
        <ChipRow>
          <Chip
            tone="accent"
            testID={`feed-language-${locale}`}
            onPress={onOpenLanguage ? () => onOpenLanguage(locale) : undefined}
          >
            {languageLabel(locale, i18n.language)}
          </Chip>
        </ChipRow>
      ) : null}
      <SplitBar
        segments={[
          {
            key: "approved",
            value: (c.approved ?? 0) + (c.bulk_approved ?? 0),
            colour: "bg-primary",
            label: t("feed.split.approved"),
          },
          { key: "fixed", value: c.fixed ?? 0, colour: "bg-prism-3", label: t("feed.split.fixed") },
          {
            key: "rejected",
            value: c.rejected ?? 0,
            colour: "bg-corrective",
            label: t("feed.split.rejected"),
          },
          {
            key: "unreviewed",
            value: c.unreviewed ?? 0,
            colour: "bg-prism-2",
            label: t("feed.split.sentBack"),
          },
        ]}
      />
    </View>
  );
}

function axesText(c: Record<string, string> | undefined): string {
  return Object.values(c ?? {}).join(" · ");
}

function RecipePreview({ entry, onOpenLanguage }: PreviewProps) {
  const { i18n } = useTranslation();
  const { added, removed } = targetChanges(entry);
  return (
    <ChipRow>
      {added.map((l) => (
        <Chip
          key={`+${l}`}
          tone="added"
          onPress={onOpenLanguage ? () => onOpenLanguage(l) : undefined}
        >
          {`+ ${languageLabel(l, i18n.language)}`}
        </Chip>
      ))}
      {removed.map((l) => (
        <Chip key={`-${l}`} tone="removed">
          {`− ${languageLabel(l, i18n.language)}`}
        </Chip>
      ))}
      {(entry.diff?.collections ?? []).map((col) => (
        <Chip key={`c-${col.name}`} tone="accent">
          {col.before && Object.keys(col.before).length > 0
            ? `${col.name}: ${axesText(col.before)} → ${axesText(col.after)}`
            : `${col.name}: ${axesText(col.after)}`}
        </Chip>
      ))}
      {(entry.diff?.settings ?? []).map((s) => (
        <Chip key={`s-${s.name}`}>{`${s.name}: ${s.after || "–"}`}</Chip>
      ))}
    </ChipRow>
  );
}

function ContextPreview({ entry }: PreviewProps) {
  const { t } = useTranslation();
  const samples = entry.samples ?? [];
  const total = CONTEXT_OPS.reduce((n, op) => n + (entry.counts[op] ?? 0), 0);
  const more = Math.max(0, total - samples.length);
  if (samples.length === 0) {
    return null;
  }
  return (
    <View className="gap-2">
      {samples.map((s, i) =>
        s.kind === "note" ? (
          <View key={`n-${i}`} className="border-prism-4 border-l-2 pl-3">
            <Text className="text-foreground text-[14px] italic" numberOfLines={3}>
              {s.text}
            </Text>
          </View>
        ) : (
          <View key={`t-${i}`} className="flex-row">
            <View className="bg-chip shrink rounded-xl px-3 py-2">
              <Text className="text-chip-foreground text-[14px]">
                <Text className="font-semibold">{s.replacement || s.term}</Text>
                {s.replacement ? (
                  <>
                    {"  ←  "}
                    <Text className="line-through opacity-80">{s.term}</Text>
                  </>
                ) : null}
              </Text>
            </View>
          </View>
        ),
      )}
      {more > 0 ? (
        <Text className="text-muted-foreground text-[13px]">
          {t("feed.moreItems", { count: more })}
        </Text>
      ) : null}
    </View>
  );
}

const STATE_TONE: Record<string, string> = {
  merged: "bg-added",
  approved: "bg-added",
  rejected: "bg-removed",
  abandoned: "bg-muted",
  superseded: "bg-muted",
};

function ChangeSetPreview({ entry }: PreviewProps) {
  const { t } = useTranslation();
  const ops = Object.entries(entry.counts).filter(([, n]) => n > 0);
  return (
    <View className="gap-2">
      <View className="flex-row items-center gap-2">
        {entry.state ? (
          <View className={`${STATE_TONE[entry.state] ?? "bg-chip"} rounded-md px-2 py-0.5`}>
            <Text className="text-foreground text-[12px] font-semibold uppercase">
              {stateLabel(t, entry.state)}
            </Text>
          </View>
        ) : null}
        {entry.name ? (
          <Text className="text-foreground flex-1 text-[14px]" numberOfLines={1}>
            {entry.name}
          </Text>
        ) : null}
      </View>
      {ops.length > 0 ? (
        <ChipRow>
          {ops.map(([op, n]) => (
            <Chip key={op}>
              {t(`feed.op.${op.replace(/\./g, "_")}`, { count: n, defaultValue: `${n} ${op}` })}
            </Chip>
          ))}
        </ChipRow>
      ) : null}
    </View>
  );
}

function ContextFallback({ entry }: PreviewProps) {
  const { t } = useTranslation();
  const kinds = Object.entries(entry.counts).filter(([k, n]) => !CONTEXT_OPS.includes(k) && n > 0);
  return (
    <ChipRow>
      {kinds.map(([k, n]) => (
        <Chip key={k} tone="accent">
          {countLabel(t, { key: k, n })}
        </Chip>
      ))}
    </ChipRow>
  );
}

/** The preview of one entry, or null for a kind with nothing to show. */
export function Preview(props: PreviewProps) {
  switch (props.entry.kind) {
    case "push":
      return <PushPreview {...props} />;
    case "convergence_run":
      return <RunPreview {...props} />;
    case "review_summary":
      return <ReviewPreview {...props} />;
    case "recipe_change":
      return <RecipePreview {...props} />;
    case "context_change":
      return (props.entry.samples?.length ?? 0) > 0 ? (
        <ContextPreview {...props} />
      ) : (
        <ContextFallback {...props} />
      );
    case "change_set":
      return <ChangeSetPreview {...props} />;
    default:
      return null;
  }
}

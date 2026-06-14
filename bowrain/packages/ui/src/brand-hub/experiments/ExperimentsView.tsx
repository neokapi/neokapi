// Experiments — change-sets in the Brand hub (AD-021). A change-set is a named,
// reviewable draft of graph + voice edits whose blast radius is measured before
// merge. This view lists them grouped by lifecycle status; "New experiment"
// opens the what-if wizard to compose one with a live blast-radius preview.
import { useMemo, useState } from "react";
import {
  Button,
  Card,
  CardContent,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@neokapi/ui-primitives";
import { Plus, FlaskConical } from "../../components/icons";
import type { ChangeSet, ChangeSetStatus } from "../../types/brand-graph";
import { CHANGE_SET_STATUSES } from "../../types/brand-graph";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { BrandHub } from "../shell/BrandHub";
import {
  ChangeSetStatusBadge,
  changeSetStatusLabel,
  EmptyState,
  formatRelative,
} from "../shell/atoms";
import { WhatIfWizard } from "./WhatIfWizard";

const ALL = "all";

// Active statuses lead; terminal states trail.
const GROUP_ORDER: ChangeSetStatus[] = ["draft", "in_review", "approved", "merged", "abandoned"];

export interface ExperimentsViewProps {
  onOpenExperiment: (changesetId: string) => void;
}

export function ExperimentsView({ onOpenExperiment }: ExperimentsViewProps) {
  const [filter, setFilter] = useState<string>(ALL);
  const [wizardOpen, setWizardOpen] = useState(false);
  const { nameOf } = useUserDisplayNames();

  const { data: changesets, isLoading } = useChangesets(
    filter === ALL ? undefined : (filter as ChangeSetStatus),
  );

  const groups = useMemo(() => {
    const map = new Map<ChangeSetStatus, ChangeSet[]>();
    for (const cs of changesets ?? []) {
      const arr = map.get(cs.status) ?? [];
      arr.push(cs);
      map.set(cs.status, arr);
    }
    return GROUP_ORDER.map((status) => ({ status, items: map.get(status) ?? [] })).filter(
      (g) => g.items.length > 0,
    );
  }, [changesets]);

  return (
    <BrandHub
      title="Experiments"
      description="Governed changes travel as change-sets — a reviewable draft whose impact on published content is measured before it merges."
      width="wide"
      actions={
        <Button size="sm" onClick={() => setWizardOpen(true)}>
          <Plus />
          New experiment
        </Button>
      }
      toolbar={
        <Select value={filter} onValueChange={setFilter}>
          <SelectTrigger className="w-44" size="sm">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All statuses</SelectItem>
            {CHANGE_SET_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {changeSetStatusLabel(s)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
    >
      {isLoading ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      ) : groups.length === 0 ? (
        <EmptyState
          icon={<FlaskConical />}
          title="No experiments yet"
          description="Start a change-set to propose a governed change — banning a term, changing a preferred term, or piloting a rule on real content."
          action={
            <Button size="sm" variant="outline" onClick={() => setWizardOpen(true)}>
              <Plus />
              New experiment
            </Button>
          }
        />
      ) : (
        <div className="space-y-6">
          {groups.map((group) => (
            <section key={group.status} className="space-y-2">
              <div className="flex items-center gap-2">
                <ChangeSetStatusBadge status={group.status} />
                <span className="text-xs text-muted-foreground">{group.items.length}</span>
              </div>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {group.items.map((cs) => (
                  <ExperimentCard
                    key={cs.id}
                    changeset={cs}
                    authorName={nameOf(cs.created_by)}
                    onOpen={() => onOpenExperiment(cs.id)}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      <WhatIfWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        onSubmitted={(id) => {
          setWizardOpen(false);
          onOpenExperiment(id);
        }}
      />
    </BrandHub>
  );
}

function ExperimentCard({
  changeset,
  authorName,
  onOpen,
}: {
  changeset: ChangeSet;
  authorName: string;
  onOpen: () => void;
}) {
  return (
    <Card
      className="cursor-pointer transition-colors hover:border-primary/40 hover:bg-muted/30"
      onClick={onOpen}
    >
      <CardContent className="space-y-2 p-4">
        <div className="flex items-start justify-between gap-2">
          <h3 className="min-w-0 truncate font-medium text-foreground">{changeset.name}</h3>
        </div>
        {changeset.description && (
          <p className="line-clamp-2 text-sm text-muted-foreground">{changeset.description}</p>
        )}
        <div className="flex items-center gap-2 pt-1 text-xs text-muted-foreground">
          <span>{authorName}</span>
          <span aria-hidden>·</span>
          <span>updated {formatRelative(changeset.updated_at)}</span>
        </div>
      </CardContent>
    </Card>
  );
}

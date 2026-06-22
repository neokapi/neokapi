import type { Meta, StoryObj } from "@storybook/react-vite";
import { Pencil, Play, Plus, Workflow } from "lucide-react";
import { Badge, Button, ConfirmDeleteButton, ItemCard, PageHeader } from "@neokapi/ui-primitives";
import { configuredFlows, ProjectKindBadge } from "./_shared";

/**
 * Prototype: configured flows list.
 *
 * Flows are no longer a top-level sidebar pillar. They are created, edited, and
 * deleted from this list, and run ad-hoc from Quick Tools. Everyone can use
 * flows; they're just managed in one place.
 */
const meta = {
  title: "Prototype/ConfiguredFlows",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function ConfiguredFlows() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl p-8">
        <PageHeader
          title="Flows"
          subtitle="Saved tool pipelines. Edit them here; run them from a project or Quick Tools."
          actions={
            <Button size="sm">
              <Plus size={14} />
              New flow
            </Button>
          }
        />

        <div className="space-y-2.5">
          {configuredFlows.map((flow) => (
            <ItemCard key={flow.name} className="flex flex-row items-start gap-3">
              <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Workflow size={17} />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{flow.name}</span>
                  <ProjectKindBadge kind={flow.kind} />
                </div>
                <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                  {flow.description}
                </p>
                <div className="mt-2 flex flex-wrap items-center gap-1.5">
                  {flow.steps.map((step, i) => (
                    <span key={step} className="flex items-center gap-1.5">
                      {i > 0 && <span className="text-xs text-muted-foreground">&rarr;</span>}
                      <Badge variant="secondary" className="text-[11px]">
                        {step}
                      </Badge>
                    </span>
                  ))}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                <Button variant="ghost" size="icon-xs" aria-label={`Run ${flow.name}`}>
                  <Play size={13} />
                </Button>
                <Button variant="ghost" size="icon-xs" aria-label={`Edit ${flow.name}`}>
                  <Pencil size={13} />
                </Button>
                <ConfirmDeleteButton onDelete={() => undefined} />
              </div>
            </ItemCard>
          ))}
        </div>

        <p className="mt-4 px-1 text-xs text-muted-foreground">
          Flows are not a sidebar pillar. They live here so anyone — content or localization — can
          compose and reuse them.
        </p>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <ConfiguredFlows />,
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ActorMark, KNOWN_ACTORS, actorLabel } from "../../components/ui/actor-mark";
import { CoordinateChip } from "../../components/ui/coordinate-chip";

/**
 * Who did something, as a small square: two letters and a hue for each
 * assistant, a neutral mark for a person and a sync. The full name is the
 * tooltip and the accessible name.
 */
const meta: Meta<typeof ActorMark> = {
  title: "Patterns/ActorMark",
  component: ActorMark,
  parameters: { layout: "padded" },
  args: { actor: "claude-code" },
};
export default meta;

type Story = StoryObj<typeof ActorMark>;

const SIZES = ["sm", "md", "lg"] as const;

function Panel({ dark, children }: { dark?: boolean; children: React.ReactNode }) {
  return (
    <div className={dark ? "dark" : undefined}>
      <div className="rounded-lg border bg-background p-4 text-foreground">
        <p className="mb-3 text-xs font-medium text-muted-foreground">{dark ? "Dark" : "Light"}</p>
        {children}
      </div>
    </div>
  );
}

/** Every known actor in every size, in both themes. */
export const EveryActor: Story = {
  name: "Every actor",
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <table className="text-sm">
            <tbody>
              {KNOWN_ACTORS.map((actor) => (
                <tr key={actor}>
                  <td className="w-36 py-1 pr-4 text-muted-foreground">{actorLabel(actor)}</td>
                  {SIZES.map((size) => (
                    <td key={size} className="px-2 py-1">
                      <ActorMark actor={actor} size={size} />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      ))}
    </div>
  ),
};

/** An actor this build has not heard of: initials from its own name, on the neutral tint. */
export const UnknownActor: Story = {
  name: "Unknown actor",
  render: () => (
    <div className="flex items-center gap-3 text-sm">
      <ActorMark actor="lab-agent" />
      <ActorMark actor="claude@studio" />
      <ActorMark actor="settle" />
      <span className="text-muted-foreground">lab-agent, claude@studio, settle</span>
    </div>
  ),
};

/** A person shows their initials, or a person icon named "You" when the record has no name. */
export const Person: Story = {
  render: () => (
    <div className="flex flex-col gap-2 text-sm">
      <div className="flex items-center gap-2">
        <ActorMark actor="person" name="Ingrid Solberg" />
        <span>Ingrid Solberg</span>
      </div>
      <div className="flex items-center gap-2">
        <ActorMark actor="person" name="Ola" />
        <span>Ola</span>
      </div>
      <div className="flex items-center gap-2">
        <ActorMark actor="person" />
        <span>You</span>
      </div>
    </div>
  ),
};

/** Marks and coordinate chips share one ramp, so a row carrying both reads as one drawing. */
export const BesideChips: Story = {
  name: "Beside coordinate chips",
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <div className="flex items-center gap-2 text-sm">
            <ActorMark actor="claude-code" />
            <ActorMark actor="codex" />
            <span className="font-medium">Acme Help Center</span>
            <CoordinateChip axis="brand" value="Fernwell" />
            <CoordinateChip axis="channel" value="support" />
            <CoordinateChip axis="language" value="nb-NO" />
          </div>
        </Panel>
      ))}
    </div>
  ),
};

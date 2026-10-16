import type { Meta, StoryObj } from "@storybook/react-vite";
import { DIGEST_KINDS, KindIcon, kindLabel } from "../../components/ui/kind-icon";

/**
 * One icon per kind of thing a project's digest shows: the four suggestion
 * themes, a disagreement, an established rule, and drift away from one.
 * Muted, except the disagreement (warning) and the established rule (success).
 */
const meta: Meta<typeof KindIcon> = {
  title: "Patterns/KindIcon",
  component: KindIcon,
  parameters: { layout: "padded" },
  args: { kind: "name" },
};
export default meta;

type Story = StoryObj<typeof KindIcon>;

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

/** Every kind with its label beside it, in both sizes and both themes. */
export const EveryKind: Story = {
  name: "Every kind",
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <ul className="flex flex-col gap-1.5 text-sm">
            {DIGEST_KINDS.map((kind) => (
              <li
                key={kind}
                className="grid grid-cols-[20px_20px_minmax(0,1fr)_auto] items-center gap-2"
              >
                <KindIcon kind={kind} />
                <KindIcon kind={kind} size="md" />
                <span>{kindLabel(kind)}</span>
                <code className="font-mono text-[10px] text-muted-foreground">{kind}</code>
              </li>
            ))}
          </ul>
        </Panel>
      ))}
    </div>
  ),
};

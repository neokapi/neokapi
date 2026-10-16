import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../../components/ui/button";
import { ActorMark, actorLabel } from "../../components/ui/actor-mark";
import { KindIcon } from "../../components/ui/kind-icon";
import type { DigestKind } from "../../components/ui/kind-icon";
import {
  ListDetail,
  ListDetailEmpty,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
} from "../../components/ui/list-detail";

/**
 * The list-and-detail shape, rebuilt from the What's new mockup: a narrow
 * list of one-line rows grouped into Needs you, Suggested and Established,
 * and a detail pane for the selected row with its actions in the toolbar.
 * Click a row, or focus the list and use the arrow keys, `j` and `k`, Home
 * and End.
 */
const meta: Meta<typeof ListDetail> = {
  title: "Patterns/ListDetail",
  component: ListDetail,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ListDetail>;

// ── Sample data ──────────────────────────────────────────────────────────────

type Group = "needs" | "suggested" | "established";

interface Side {
  write: string;
  not: string;
  quote: string;
  path: string;
  actor: string;
  how: string;
}

interface Row {
  id: string;
  group: Group;
  kind: DigestKind;
  /** What the rule says to write, and what it replaces. */
  write: string;
  not?: string;
  actor: string;
  /** A person's name, for a person's row. */
  name?: string;
  quote?: string;
  path?: string;
  when?: string;
  /** The two sides of a disagreement. */
  sides?: [Side, Side];
}

const ROWS: Row[] = [
  {
    id: "c1",
    group: "needs",
    kind: "conflict",
    write: "sign in",
    not: "log in",
    actor: "codex",
    sides: [
      {
        write: "sign in",
        not: "log in",
        quote: "Your session has expired. Please sign in again to continue.",
        path: "src/en/error-messages.properties",
        actor: "claude-code",
        how: "from a correction a person made",
      },
      {
        write: "log in",
        not: "sign in",
        quote: "Please verify your email address before signing in.",
        path: "src/en/error-messages.properties",
        actor: "codex",
        how: "1 against",
      },
    ],
  },
  {
    id: "s1",
    group: "suggested",
    kind: "name",
    write: "KapiMart",
    not: "Kapi Mart",
    actor: "claude-code",
    quote: "Welcome to Kapi Mart, the store for everything you need.",
    path: "src/en/home.properties",
    when: "Wed",
  },
  {
    id: "s2",
    group: "suggested",
    kind: "name",
    write: "PayPal",
    not: "Paypal",
    actor: "codex",
    quote: "Pay with Paypal or a card.",
    path: "src/en/checkout.properties",
    when: "Mon",
  },
  {
    id: "s3",
    group: "suggested",
    kind: "word",
    write: "shopping cart",
    not: "basket",
    actor: "claude-code",
    quote: "Your basket is empty.",
    path: "src/en/cart.properties",
    when: "Wed",
  },
  {
    id: "s4",
    group: "suggested",
    kind: "word",
    write: "sandbox",
    not: "test mode",
    actor: "claude-code",
    quote: "Payments in test mode are not charged.",
    path: "src/en/checkout.properties",
    when: "Wed",
  },
  {
    id: "s5",
    group: "suggested",
    kind: "note",
    write: "Address the shopper as you",
    actor: "claude-code",
    quote: "You can change your delivery address at any time.",
    path: "src/en/account.properties",
    when: "Wed",
  },
  {
    id: "s6",
    group: "suggested",
    kind: "memory",
    write: "handlekurv",
    not: "shopping cart",
    actor: "codex",
    quote: "Handlekurven din er tom.",
    path: "src/nb/cart.properties",
    when: "today",
  },
  {
    id: "e1",
    group: "established",
    kind: "established",
    write: "studio",
    not: "business",
    actor: "person",
    name: "Ingrid Solberg",
    quote: "Set up your studio in a few steps.",
    path: "src/en/onboarding.properties",
    when: "3 Oct",
  },
  {
    id: "e2",
    group: "established",
    kind: "established",
    write: "checkout",
    not: "check-out",
    actor: "person",
    quote: "Go to checkout",
    path: "src/en/cart.properties",
    when: "1 Oct",
  },
  {
    id: "e3",
    group: "established",
    kind: "established",
    write: "sign up",
    not: "register",
    actor: "person",
    name: "Ingrid Solberg",
    quote: "Sign up to save your cart.",
    path: "src/en/account.properties",
    when: "28 Sep",
  },
];

const GROUPS: Array<{ id: Group; label: string }> = [
  { id: "needs", label: "Needs you" },
  { id: "suggested", label: "Suggested" },
  { id: "established", label: "Established" },
];

// ── Rows and details ─────────────────────────────────────────────────────────

function RowLine({ row }: { row: Row }) {
  if (row.kind === "conflict") {
    return (
      <>
        <span className="font-medium">{row.write}</span> or{" "}
        <span className="font-medium">{row.not}</span>
      </>
    );
  }
  if (!row.not) return <>{row.write}</>;
  return (
    <>
      <span className="font-medium">{row.write}</span>, not{" "}
      <s className="text-muted-foreground">{row.not}</s>
    </>
  );
}

function rowTrailing(row: Row) {
  if (row.sides) {
    return (
      <>
        {row.sides.map((side) => (
          <ActorMark key={side.actor} actor={side.actor} size="sm" />
        ))}
      </>
    );
  }
  return <ActorMark actor={row.actor} name={row.name} size="sm" />;
}

function Quote({ text, path }: { text: string; path: string }) {
  return (
    <div className="flex flex-col gap-1 border-l-2 border-border pl-3">
      <p className="text-sm italic">“{text}”</p>
      <p className="font-mono text-[10px] text-muted-foreground">{path}</p>
    </div>
  );
}

function NoticedBy({ actor, name, how }: { actor: string; name?: string; how: string }) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 text-[11px]">
      <span className="text-muted-foreground">Noticed by</span>
      <span className="flex items-center gap-1.5 text-sm">
        <ActorMark actor={actor} name={name} size="sm" decorative />
        <span>
          {name ?? actorLabel(actor)}
          <span className="text-muted-foreground"> · {how}</span>
        </span>
      </span>
    </div>
  );
}

function ConflictDetail({ row }: { row: Row & { sides: [Side, Side] } }) {
  return (
    <ListDetailPane
      toolbar={
        <ListDetailToolbar
          title={
            <>
              {row.write} <span className="font-normal text-muted-foreground">or</span> {row.not}?
            </>
          }
          actions={
            <span className="text-[11px] text-muted-foreground">
              Two assistants disagree. Choose one and the other is set aside.
            </span>
          }
        />
      }
    >
      <div className="@container">
        <div className="grid gap-3 @md:grid-cols-2">
          {row.sides.map((side, i) => (
            <div
              key={side.actor}
              className="flex flex-col gap-3 rounded-lg border border-border p-3"
            >
              <p className="text-sm font-semibold">
                Write {side.write},{" "}
                <s className="font-normal text-muted-foreground">not {side.not}</s>
              </p>
              <Quote text={side.quote} path={side.path} />
              <NoticedBy actor={side.actor} how={side.how} />
              <div className="flex flex-wrap items-center gap-1.5">
                <Button size="sm" variant={i === 0 ? "default" : "outline"}>
                  Choose this side
                </Button>
                <Button size="sm" variant="ghost">
                  Change
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </ListDetailPane>
  );
}

function SuggestionDetail({ row }: { row: Row }) {
  const established = row.group === "established";
  return (
    <ListDetailPane
      toolbar={
        <ListDetailToolbar
          title={
            <>
              {row.write}
              {row.not && (
                <>
                  , <s className="font-normal text-muted-foreground">not {row.not}</s>
                </>
              )}
            </>
          }
          actions={
            established ? (
              <>
                <Button size="sm" variant="outline">
                  Apply more widely
                </Button>
                <Button size="sm" variant="ghost">
                  Drop
                </Button>
              </>
            ) : (
              <>
                <Button size="sm">Keep</Button>
                <Button size="sm" variant="outline">
                  Change
                </Button>
                <Button size="sm" variant="ghost">
                  Drop
                </Button>
              </>
            )
          }
        />
      }
    >
      {row.quote && row.path && <Quote text={row.quote} path={row.path} />}
      <NoticedBy
        actor={row.actor}
        name={row.name}
        how={established ? `kept ${row.when}` : (row.when ?? "")}
      />
    </ListDetailPane>
  );
}

function Detail({ row }: { row: Row | undefined }) {
  if (!row) {
    return (
      <ListDetailEmpty
        title="Choose a suggestion"
        description="Its wording, where it was seen and who noticed it appear here."
      />
    );
  }
  if (row.sides) return <ConflictDetail row={row as Row & { sides: [Side, Side] }} />;
  return <SuggestionDetail row={row} />;
}

// ── The page ─────────────────────────────────────────────────────────────────

function WhatsNew({
  initial,
  rows = ROWS,
  width,
  height = 460,
}: {
  initial: string | null;
  rows?: Row[];
  width?: number;
  height?: number;
}) {
  const [selected, setSelected] = useState<string | null>(initial);
  return (
    <div
      className="overflow-hidden rounded-lg border border-border bg-background text-foreground"
      style={{ height, width }}
    >
      <ListDetail
        list={
          <ListDetailList selectedId={selected} onSelect={setSelected} aria-label="Suggestions">
            {GROUPS.map((group) => {
              const items = rows.filter((r) => r.group === group.id);
              if (items.length === 0) return null;
              return (
                <ListDetailGroup key={group.id} label={group.label} count={items.length}>
                  {items.map((row) => (
                    <ListDetailItem
                      key={row.id}
                      id={row.id}
                      leading={<KindIcon kind={row.kind} />}
                      trailing={rowTrailing(row)}
                    >
                      <RowLine row={row} />
                    </ListDetailItem>
                  ))}
                </ListDetailGroup>
              );
            })}
          </ListDetailList>
        }
        detail={<Detail row={rows.find((r) => r.id === selected)} />}
      />
    </div>
  );
}

/** The disagreement selected: two sides, each with its evidence and who noticed it. */
export const Conflict: Story = {
  name: "What's new: a disagreement",
  render: () => <WhatsNew initial="c1" />,
};

/** A plain suggestion selected, with Keep, Change and Drop in the toolbar. */
export const Suggestion: Story = {
  name: "What's new: a suggestion",
  render: () => <WhatsNew initial="s1" />,
};

/** Nothing selected: the detail pane says what would appear there. */
export const NothingSelected: Story = {
  name: "What's new: nothing selected",
  render: () => <WhatsNew initial={null} />,
};

function selectedRow(canvasElement: HTMLElement) {
  return within(canvasElement)
    .getByRole("listbox", { name: "Suggestions" })
    .querySelector('[role="option"][aria-selected="true"]');
}

/** The keyboard path: arrows and j/k move the selection, Home and End jump, group headers are skipped. */
export const Keyboard: Story = {
  render: () => <WhatsNew initial={null} />,
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);
    const listbox = () => canvas.getByRole("listbox", { name: "Suggestions" });

    await step("Focus the list; nothing is selected yet", async () => {
      listbox().focus();
      await expect(selectedRow(canvasElement)).toBeNull();
    });
    await step("ArrowDown selects the first row", async () => {
      await userEvent.keyboard("{ArrowDown}");
      await expect(selectedRow(canvasElement)).toHaveTextContent("sign in or log in");
    });
    await step("j crosses the group header to the next group's first row", async () => {
      await userEvent.keyboard("j");
      await expect(selectedRow(canvasElement)).toHaveTextContent("KapiMart, not Kapi Mart");
      await expect(canvas.getByRole("heading", { level: 2 })).toHaveTextContent(
        "KapiMart, not Kapi Mart",
      );
    });
    await step("End jumps to the last row and k moves back one", async () => {
      await userEvent.keyboard("{End}");
      await expect(selectedRow(canvasElement)).toHaveTextContent("sign up, not register");
      await userEvent.keyboard("k");
      await expect(selectedRow(canvasElement)).toHaveTextContent("checkout, not check-out");
    });
    await step("Home jumps to the first row, named by aria-activedescendant", async () => {
      await userEvent.keyboard("{Home}");
      const row = selectedRow(canvasElement);
      await expect(row).toHaveTextContent("sign in or log in");
      await expect(listbox()).toHaveAttribute("aria-activedescendant", row?.id);
    });
  },
};

/** Under 760px the list stacks above the detail and scrolls in a shorter pane. */
export const NarrowPane: Story = {
  name: "Narrow pane",
  render: () => <WhatsNew initial="s3" width={520} height={620} />,
};

const LONG_ROWS: Row[] = [
  {
    ...ROWS[1],
    id: "long",
    write: "Fernwell Professional Services Cloud Integration Platform",
    not: "Fernwell professional services cloud-integration platform",
  },
  ...Array.from({ length: 30 }, (_, i): Row => {
    const base = ROWS[1 + (i % 6)];
    return { ...base, id: `${base.id}-${i}` };
  }),
  ...ROWS.filter((r) => r.group === "established"),
];

/** A long row truncates with an ellipsis; a long list scrolls under its sticky group headers. */
export const LongContent: Story = {
  name: "Long content",
  render: () => <WhatsNew initial="long" rows={LONG_ROWS} />,
};

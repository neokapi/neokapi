import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ArrowRight, Check, Languages, ShieldCheck } from "lucide-react";
import { Button, LocalePill } from "@neokapi/ui-primitives";
import { Chip, JourneyCard } from "./_shared";

/**
 * Prototype: new-project journey selection.
 *
 * The two journeys expanded with what each scaffolds. Choosing a journey is an
 * explicit first step; "Continue" advances to naming and configuration.
 */
const meta = {
  title: "Prototype/NewProject",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

function ScaffoldItem({ children, muted }: { children: React.ReactNode; muted?: boolean }) {
  return (
    <li className="flex items-center gap-2 text-xs">
      <Check size={13} className={muted ? "text-muted-foreground/40" : "text-primary"} />
      <span className={muted ? "text-muted-foreground/60 line-through" : "text-muted-foreground"}>
        {children}
      </span>
    </li>
  );
}

function NewProject() {
  const [choice, setChoice] = useState<"brand" | "localize">("brand");

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-3xl">
        <div className="mb-6 text-center">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Step 1 of 2 &middot; Choose a journey
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">What are you setting up?</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Both start from your local files. You can add localization to a content project later.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <JourneyCard
            icon={<ShieldCheck size={22} />}
            eyebrow="Content"
            title="Keep content on brand"
            description="Brand voice and quality checks for content that stays in one language."
            selected={choice === "brand"}
            onClick={() => setChoice("brand")}
            chips={
              <>
                <Chip>No target languages</Chip>
              </>
            }
            footer={
              <ul className="space-y-1.5">
                <ScaffoldItem>Brand voice profile (brand_voice)</ScaffoldItem>
                <ScaffoldItem>Brand &amp; terminology checks</ScaffoldItem>
                <ScaffoldItem>Content collections</ScaffoldItem>
                <ScaffoldItem muted>Target languages &amp; translation</ScaffoldItem>
              </ul>
            }
          />
          <JourneyCard
            icon={<Languages size={22} />}
            eyebrow="Localization"
            title="Localize content"
            description="Everything in the content journey, plus languages and the translation surface."
            selected={choice === "localize"}
            onClick={() => setChoice("localize")}
            chips={
              <>
                <LocalePill locale="en-US" />
                <span className="text-muted-foreground">&rarr;</span>
                <LocalePill locale="fr-FR" />
                <LocalePill locale="de-DE" />
              </>
            }
            footer={
              <ul className="space-y-1.5">
                <ScaffoldItem>Source &amp; target languages</ScaffoldItem>
                <ScaffoldItem>Translate + QA flow</ScaffoldItem>
                <ScaffoldItem>Translation memory &amp; termbase</ScaffoldItem>
                <ScaffoldItem>Brand voice &amp; checks</ScaffoldItem>
              </ul>
            }
          />
        </div>

        <div className="mt-6 flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {choice === "brand"
              ? "A content project — no translation."
              : "A localization project — content tooling plus languages."}
          </span>
          <Button>
            Continue
            <ArrowRight size={15} />
          </Button>
        </div>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <NewProject />,
};

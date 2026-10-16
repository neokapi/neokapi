// A ladder of steps drawn as dots on a connector line, each with its name
// under it. A step is done, current or still to do, and the line up to the
// current step takes the primary colour.
//
// Every step is a button. `value` names the step whose detail the page shows,
// which can differ from the current one when a reader clicks ahead to read
// about a later step. The steps form a radio group: one tab stop, and the
// arrow keys move the selection. Below 480 pixels of width the steps stack,
// with the connector down the start edge.

import * as React from "react";
import { Check } from "lucide-react";

import { cn } from "../../lib/utils";

/** Where a step stands: reached, the one the subject is on, or ahead of it. */
export type StepState = "done" | "current" | "todo";

export interface StepperStep {
  /** The value `onChange` reports and `value` names. */
  id: string;
  /** The step's name, drawn under its dot. The caller passes it translated. */
  label: React.ReactNode;
  state: StepState;
}

/** Attributes for one step's button, such as the caller's own `data-slot`. */
export type StepperStepAttributes = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  [key: `data-${string}`]: string | number | boolean | undefined;
};

export interface StepperProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "onChange"> {
  steps: StepperStep[];
  /** The step whose detail the page shows. Absent, the current step. */
  value?: string;
  /** Called with a step's id when the reader clicks it or moves to it by key. */
  onChange?: (id: string) => void;
  /** Extra attributes for each step's button. */
  stepAttributes?: (step: StepperStep, index: number) => StepperStepAttributes | undefined;
}

/** The index the connector is drawn in the primary colour up to. */
export function stepperReach(steps: StepperStep[]): number {
  const current = steps.findIndex((s) => s.state === "current");
  if (current >= 0) return current;
  let done = -1;
  steps.forEach((s, i) => {
    if (s.state === "done") done = i;
  });
  return done;
}

// The two halves of the connector around a dot: down the track's centre
// when the steps stack, along it from the inline start when they sit in a row.
const HALF = "absolute bg-border";
const DOWN_BEFORE = "inset-x-0 top-0 bottom-1/2 mx-auto w-0.5 @min-[480px]:hidden";
const DOWN_AFTER = "inset-x-0 top-1/2 bottom-0 mx-auto w-0.5 @min-[480px]:hidden";
const ALONG_BEFORE = "inset-y-0 start-0 end-1/2 my-auto hidden h-0.5 @min-[480px]:block";
const ALONG_AFTER = "inset-y-0 start-1/2 end-0 my-auto hidden h-0.5 @min-[480px]:block";

const DOT: Record<StepState, string> = {
  done: "border-primary bg-primary text-primary-foreground",
  current: "border-primary bg-primary ring-4 ring-primary/15",
  todo: "border-muted-foreground/40 bg-background",
};

const LABEL: Record<StepState, string> = {
  done: "text-foreground",
  current: "font-medium text-foreground",
  todo: "text-muted-foreground group-hover/step:text-foreground",
};

function Stepper({ steps, value, onChange, stepAttributes, className, ...props }: StepperProps) {
  const buttons = React.useRef<Array<HTMLButtonElement | null>>([]);
  const reach = stepperReach(steps);
  const selected =
    steps.find((s) => s.id === value)?.id ??
    steps.find((s) => s.state === "current")?.id ??
    steps[0]?.id;

  const onKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    const rtl = getComputedStyle(event.currentTarget).direction === "rtl";
    let next: number;
    switch (event.key) {
      case "ArrowRight":
        next = index + (rtl ? -1 : 1);
        break;
      case "ArrowLeft":
        next = index + (rtl ? 1 : -1);
        break;
      case "ArrowDown":
        next = index + 1;
        break;
      case "ArrowUp":
        next = index - 1;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = steps.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    const to = (next + steps.length) % steps.length;
    buttons.current[to]?.focus();
    onChange?.(steps[to].id);
  };

  return (
    <div role="radiogroup" data-slot="stepper" className={cn("@container", className)} {...props}>
      <div className="flex flex-col @min-[480px]:flex-row">
        {steps.map((step, index) => (
          <StepButton
            key={step.id}
            ref={(el) => {
              buttons.current[index] = el;
            }}
            label={step.label}
            state={step.state}
            selected={step.id === selected}
            first={index === 0}
            last={index === steps.length - 1}
            litBefore={index <= reach}
            litAfter={index < reach}
            onClick={() => onChange?.(step.id)}
            onKeyDown={(event) => onKeyDown(event, index)}
            attributes={stepAttributes?.(step, index)}
          />
        ))}
      </div>
    </div>
  );
}

interface StepButtonProps {
  ref: React.Ref<HTMLButtonElement>;
  label: React.ReactNode;
  state: StepState;
  selected: boolean;
  first: boolean;
  last: boolean;
  litBefore: boolean;
  litAfter: boolean;
  onClick: () => void;
  onKeyDown: (event: React.KeyboardEvent<HTMLButtonElement>) => void;
  attributes?: StepperStepAttributes;
}

/** One step: its dot on the connector, and its name. */
function StepButton({
  ref,
  label,
  state,
  selected,
  first,
  last,
  litBefore,
  litAfter,
  onClick,
  onKeyDown,
  attributes,
}: StepButtonProps) {
  return (
    <button
      ref={ref}
      type="button"
      role="radio"
      aria-checked={selected}
      aria-current={state === "current" ? "step" : undefined}
      tabIndex={selected ? 0 : -1}
      data-slot="stepper-step"
      data-state={state}
      data-selected={selected || undefined}
      onClick={onClick}
      onKeyDown={onKeyDown}
      className={cn(
        "group/step relative flex min-w-0 flex-1 items-stretch gap-3 rounded-md text-start outline-none",
        "focus-visible:ring-2 focus-visible:ring-ring/50",
        "@min-[480px]:flex-col @min-[480px]:items-center @min-[480px]:gap-2 @min-[480px]:pb-1 @min-[480px]:text-center",
      )}
      {...attributes}
    >
      <span
        aria-hidden
        className="relative flex w-4 shrink-0 items-center justify-center @min-[480px]:h-4 @min-[480px]:w-full"
      >
        {!first && (
          <>
            <span className={cn(HALF, DOWN_BEFORE, litBefore && "bg-primary")} />
            <span className={cn(HALF, ALONG_BEFORE, litBefore && "bg-primary")} />
          </>
        )}
        {!last && (
          <>
            <span className={cn(HALF, DOWN_AFTER, litAfter && "bg-primary")} />
            <span className={cn(HALF, ALONG_AFTER, litAfter && "bg-primary")} />
          </>
        )}
        <span
          className={cn(
            "relative z-10 flex size-4 items-center justify-center rounded-full border-2",
            DOT[state],
          )}
        >
          {state === "done" && <Check className="size-2.5" strokeWidth={3.5} />}
        </span>
      </span>
      <span
        className={cn(
          "min-w-0 py-2 text-sm @min-[480px]:px-2 @min-[480px]:py-0",
          LABEL[state],
          selected && "underline decoration-primary decoration-2 underline-offset-[6px]",
        )}
      >
        {label}
      </span>
    </button>
  );
}

export { Stepper };

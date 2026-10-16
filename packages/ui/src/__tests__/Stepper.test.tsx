// @vitest-environment jsdom
import { useState } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Stepper, stepperReach, type StepState, type StepperStep } from "../components/ui/stepper";

afterEach(cleanup);

function ladder(states: StepState[]): StepperStep[] {
  const names = ["On this machine", "Shared with your team", "Managed on a server"];
  return states.map((state, i) => ({
    id: ["local", "team", "managed"][i],
    label: names[i],
    state,
  }));
}

/** A stepper that keeps its own selection, as a page does. */
function Controlled({
  states,
  onChange,
}: {
  states: StepState[];
  onChange?: (id: string) => void;
}) {
  const [value, setValue] = useState<string | undefined>(undefined);
  return (
    <Stepper
      aria-label="How far this context reaches"
      steps={ladder(states)}
      value={value}
      onChange={(id) => {
        setValue(id);
        onChange?.(id);
      }}
    />
  );
}

const radios = () => screen.getAllByRole("radio");
const checked = () =>
  radios()
    .filter((r) => r.getAttribute("aria-checked") === "true")
    .map((r) => r.textContent);

describe("Stepper", () => {
  it("is a named radio group with one radio per step", () => {
    render(<Controlled states={["current", "todo", "todo"]} />);
    const group = screen.getByRole("radiogroup", { name: "How far this context reaches" });
    expect(group.dataset.slot).toBe("stepper");
    expect(radios().map((r) => r.textContent)).toEqual([
      "On this machine",
      "Shared with your team",
      "Managed on a server",
    ]);
    expect(radios().map((r) => r.dataset.state)).toEqual(["current", "todo", "todo"]);
  });

  it("marks the current step with aria-current and selects it when no value is given", () => {
    render(<Stepper aria-label="Reach" steps={ladder(["done", "current", "todo"])} />);
    const [first, second, third] = radios();
    expect(first.getAttribute("aria-current")).toBeNull();
    expect(second.getAttribute("aria-current")).toBe("step");
    expect(third.getAttribute("aria-current")).toBeNull();
    expect(checked()).toEqual(["Shared with your team"]);
  });

  it("keeps one tab stop, on the selected step", () => {
    render(
      <Stepper aria-label="Reach" steps={ladder(["current", "todo", "todo"])} value="managed" />,
    );
    expect(radios().map((r) => r.tabIndex)).toEqual([-1, -1, 0]);
    expect(checked()).toEqual(["Managed on a server"]);
    // The current step stays current while another is selected.
    expect(radios()[0].getAttribute("aria-current")).toBe("step");
  });

  it("calls onChange with the clicked step's id", async () => {
    const onChange = vi.fn();
    render(<Controlled states={["current", "todo", "todo"]} onChange={onChange} />);
    await userEvent.click(screen.getByRole("radio", { name: "Managed on a server" }));
    expect(onChange).toHaveBeenCalledWith("managed");
    expect(checked()).toEqual(["Managed on a server"]);
  });

  it("moves the selection with the arrow keys, wrapping at the ends, as a radio group does", async () => {
    const onChange = vi.fn();
    render(<Controlled states={["current", "todo", "todo"]} onChange={onChange} />);
    radios()[0].focus();

    await userEvent.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith("team");
    expect(document.activeElement).toBe(radios()[1]);
    expect(checked()).toEqual(["Shared with your team"]);

    await userEvent.keyboard("{ArrowDown}");
    expect(onChange).toHaveBeenLastCalledWith("managed");
    await userEvent.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith("local");
    expect(document.activeElement).toBe(radios()[0]);

    await userEvent.keyboard("{ArrowLeft}");
    expect(onChange).toHaveBeenLastCalledWith("managed");
    await userEvent.keyboard("{ArrowUp}");
    expect(onChange).toHaveBeenLastCalledWith("team");
    await userEvent.keyboard("{Home}");
    expect(onChange).toHaveBeenLastCalledWith("local");
    await userEvent.keyboard("{End}");
    expect(onChange).toHaveBeenLastCalledWith("managed");
    expect(document.activeElement).toBe(radios()[2]);
  });

  it("leaves other keys to the page", async () => {
    const onChange = vi.fn();
    render(<Controlled states={["current", "todo", "todo"]} onChange={onChange} />);
    radios()[0].focus();
    await userEvent.keyboard("a");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("draws a check mark on a done step only", () => {
    render(<Stepper aria-label="Reach" steps={ladder(["done", "current", "todo"])} />);
    const [done, current, todo] = radios();
    expect(done.querySelector("svg")).not.toBeNull();
    expect(current.querySelector("svg")).toBeNull();
    expect(todo.querySelector("svg")).toBeNull();
  });

  it("passes a caller's attributes to each step", () => {
    render(
      <Stepper
        aria-label="Reach"
        data-slot="share-ladder"
        steps={ladder(["done", "current", "todo"])}
        stepAttributes={(step) => ({
          "data-slot": "share-rung",
          "data-rung": step.id,
          "data-current": step.state === "current" || undefined,
        })}
      />,
    );
    expect(screen.getByRole("radiogroup").dataset.slot).toBe("share-ladder");
    expect(radios().map((r) => r.dataset.slot)).toEqual(["share-rung", "share-rung", "share-rung"]);
    const current = document.querySelectorAll<HTMLElement>(
      "[data-slot='share-rung'][data-current]",
    );
    expect(Array.from(current).map((el) => el.dataset.rung)).toEqual(["team"]);
  });
});

describe("stepperReach", () => {
  it("is the current step, or the last done one when none is current", () => {
    expect(stepperReach(ladder(["current", "todo", "todo"]))).toBe(0);
    expect(stepperReach(ladder(["done", "todo", "current"]))).toBe(2);
    expect(stepperReach(ladder(["done", "done", "done"]))).toBe(2);
    expect(stepperReach(ladder(["todo", "todo", "todo"]))).toBe(-1);
  });
});

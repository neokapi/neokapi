"use client";

import * as React from "react";
import { createPortal } from "react-dom";
import { t } from "@neokapi/i18n-react/runtime";
import { XIcon } from "lucide-react";

import { cn } from "../../lib/utils";
import { usePortalThemeClass } from "../../lib/portal-theme";
import { Button } from "../ui/button";
import { Markdown } from "../ui/markdown";
import {
  type Box,
  type BubblePosition,
  type Size,
  cutoutBox,
  isInViewport,
  placeBubble,
} from "./geometry";
import type { Guide } from "./types";
import { queryTarget, waitForTarget } from "./wait-for-target";

/**
 * The overlay sits above Radix sheets, dialogs and popovers, which all use
 * `z-50`, and above toasts.
 */
export const GUIDE_OVERLAY_Z_INDEX = 1000;

/** The default time a step waits for its target after `before`, in ms. */
export const DEFAULT_WAIT_FOR = 4000;

export interface TourOverlayProps {
  guide: Guide;
  step: number;
  onNext: () => void;
  onBack: () => void;
  onStop: () => void;
}

type StepState = { status: "preparing" } | { status: "ready"; element: Element | null };

function viewportSize(): Size {
  return { width: window.innerWidth, height: window.innerHeight };
}

function prefersReducedMotion(): boolean {
  try {
    return window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
  } catch {
    return false;
  }
}

function toBox(rect: DOMRect): Box {
  return { top: rect.top, left: rect.left, width: rect.width, height: rect.height };
}

function sameBox(a: Box | null, b: Box | null): boolean {
  if (a === b) return true;
  if (!a || !b) return false;
  return a.top === b.top && a.left === b.left && a.width === b.width && a.height === b.height;
}

function isEditable(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  return (
    el instanceof HTMLInputElement ||
    el instanceof HTMLTextAreaElement ||
    el instanceof HTMLSelectElement
  );
}

/**
 * The running tour: a dim layer with a cutout around the step's target and a
 * bubble beside it. It runs the step's `before`, waits for the target, keeps
 * the cutout on the target while it moves or resizes, and handles the
 * keyboard. Rendered by `GuideProvider` while a tour runs.
 */
export function TourOverlay({ guide, step, onNext, onBack, onStop }: TourOverlayProps) {
  const current = guide.steps[step];
  const total = guide.steps.length;
  const portalThemeClass = usePortalThemeClass();
  const titleId = React.useId();
  const bodyId = React.useId();
  const bubbleRef = React.useRef<HTMLDivElement>(null);
  const [state, setState] = React.useState<StepState>({ status: "preparing" });
  const [rect, setRect] = React.useState<Box | null>(null);
  const [bubbleSize, setBubbleSize] = React.useState<Size>({ width: 0, height: 0 });
  const [viewport, setViewport] = React.useState<Size>(viewportSize);
  const reducedMotion = React.useMemo(prefersReducedMotion, []);

  // The way the tour last moved, so an optional step with nothing to point
  // at is passed over in that direction.
  const direction = React.useRef<"next" | "back">("next");
  const goNext = React.useCallback(() => {
    direction.current = "next";
    onNext();
  }, [onNext]);
  const goBack = React.useCallback(() => {
    direction.current = "back";
    onBack();
  }, [onBack]);

  // Callbacks read through a ref so listeners stay attached across renders.
  const handlers = React.useRef({ onNext: goNext, onBack: goBack, onStop });
  handlers.current = { onNext: goNext, onBack: goBack, onStop };

  // Focus returns to whatever held it when the tour began.
  React.useEffect(() => {
    const previous = document.activeElement;
    return () => {
      if (previous instanceof HTMLElement && previous.isConnected) {
        previous.focus({ preventScroll: true });
      }
    };
  }, []);

  // Prepare each step: run `before`, then wait for the target.
  React.useEffect(() => {
    if (!current) return;
    const controller = new AbortController();
    setState({ status: "preparing" });
    void (async () => {
      try {
        await current.before?.();
      } catch (err) {
        console.error(`guide ${guide.id}: step ${current.id} before() failed`, err);
      }
      if (controller.signal.aborted) return;
      const element = current.target
        ? await waitForTarget(
            current.target,
            current.waitFor ?? DEFAULT_WAIT_FOR,
            controller.signal,
          )
        : null;
      if (controller.signal.aborted) return;
      if (!element && current.target && current.optional) {
        if (direction.current === "back" && step > 0) handlers.current.onBack();
        else handlers.current.onNext();
        return;
      }
      if (element && !isInViewport(toBox(element.getBoundingClientRect()), viewportSize())) {
        element.scrollIntoView?.({
          block: "center",
          inline: "nearest",
          behavior: reducedMotion ? "auto" : "smooth",
        });
      }
      setState({ status: "ready", element });
    })();
    return () => controller.abort();
  }, [guide.id, current, step, reducedMotion]);

  const element = state.status === "ready" ? state.element : null;
  const ready = state.status === "ready";

  // Track the target's rect through resizes, scrolls and layout shifts.
  React.useEffect(() => {
    if (!ready) return;
    if (!element) {
      setRect(null);
      return;
    }
    let target: Element | null = element;
    let frame = 0;
    const measure = () => {
      frame = 0;
      if (target && !target.isConnected && current?.target) target = queryTarget(current.target);
      const next = target ? toBox(target.getBoundingClientRect()) : null;
      setRect((prev) => (sameBox(prev, next) ? prev : next));
      setViewport((prev) => {
        const v = viewportSize();
        return prev.width === v.width && prev.height === v.height ? prev : v;
      });
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    measure();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
    observer?.observe(element);
    observer?.observe(document.documentElement);
    window.addEventListener("scroll", schedule, true);
    window.addEventListener("resize", schedule);
    // A short settle loop follows the target through a smooth scroll or an
    // animated layout change that fires no event on the target itself.
    let settle = 0;
    let settleFrame = 0;
    const tick = () => {
      schedule();
      if (++settle < 60) settleFrame = requestAnimationFrame(tick);
    };
    settleFrame = requestAnimationFrame(tick);
    return () => {
      observer?.disconnect();
      window.removeEventListener("scroll", schedule, true);
      window.removeEventListener("resize", schedule);
      cancelAnimationFrame(frame);
      cancelAnimationFrame(settleFrame);
    };
  }, [ready, element, current?.target]);

  // Measure the bubble so placement can flip and clamp.
  React.useLayoutEffect(() => {
    const el = bubbleRef.current;
    if (!el) return;
    const measure = () => {
      const next = { width: el.offsetWidth, height: el.offsetHeight };
      setBubbleSize((prev) =>
        prev.width === next.width && prev.height === next.height ? prev : next,
      );
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [ready, step]);

  // Move focus to the bubble when a step is shown.
  React.useEffect(() => {
    if (ready) bubbleRef.current?.focus({ preventScroll: true });
  }, [ready, step]);

  // A step that advances on the target advances after the click reaches it.
  React.useEffect(() => {
    if (!ready || !element || current?.advanceOn !== "target") return;
    const onClick = (event: MouseEvent) => {
      if (event.target instanceof Node && element.contains(event.target)) {
        setTimeout(() => handlers.current.onNext(), 0);
      }
    };
    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, [ready, element, current?.advanceOn]);

  // Keyboard, in the capture phase so it runs before a dialog under the tour.
  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        handlers.current.onStop();
        return;
      }
      if (isEditable(event.target)) return;
      if (event.key === "ArrowRight") {
        event.preventDefault();
        event.stopPropagation();
        handlers.current.onNext();
      } else if (event.key === "ArrowLeft") {
        event.preventDefault();
        event.stopPropagation();
        handlers.current.onBack();
      } else if (event.key === "Enter") {
        // Enter on a button or link activates that control instead.
        const control =
          event.target instanceof HTMLElement && event.target.closest("button, a, [role=button]");
        if (control) return;
        event.preventDefault();
        event.stopPropagation();
        handlers.current.onNext();
      }
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, []);

  if (!current || typeof document === "undefined") return null;

  const cutout = rect ? cutoutBox(rect) : null;
  const position: BubblePosition = placeBubble(
    cutout,
    bubbleSize,
    viewport,
    current.placement ?? "bottom",
  );
  const passThrough = current.advanceOn === "target" && ready && !!cutout;
  const isLast = step === total - 1;
  const motion = reducedMotion ? "" : "motion-safe:transition-all motion-safe:duration-200";

  return createPortal(
    <div
      data-slot="guide-overlay"
      className={cn(portalThemeClass, "fixed inset-0")}
      style={{ zIndex: GUIDE_OVERLAY_Z_INDEX, pointerEvents: "none" }}
      // Pointer events stop here, so a dialog under the tour does not read a
      // press on the bubble as a press outside itself.
      onPointerDown={(event) => event.stopPropagation()}
    >
      {cutout ? (
        <>
          <div
            data-slot="guide-cutout"
            aria-hidden
            className={cn("absolute rounded-lg outline-2 outline-primary/70", motion)}
            style={{
              top: cutout.top,
              left: cutout.left,
              width: cutout.width,
              height: cutout.height,
              boxShadow: "0 0 0 200vmax rgb(0 0 0 / 0.5)",
            }}
          />
          <ClickBlockers cutout={cutout} passThrough={passThrough} />
        </>
      ) : (
        <div
          data-slot="guide-dim"
          aria-hidden
          className="absolute inset-0 bg-black/50"
          style={{ pointerEvents: "auto" }}
        />
      )}

      <div
        ref={bubbleRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={bodyId}
        tabIndex={-1}
        data-slot="guide-bubble"
        data-placement={position.placement}
        data-state={ready ? "ready" : "preparing"}
        className={cn(
          "absolute flex w-[22rem] max-w-[calc(100vw-1rem)] flex-col gap-2 rounded-xl border bg-popover p-4 text-sm text-popover-foreground shadow-lg outline-none",
          reducedMotion ? "" : "motion-safe:transition-[top,left,opacity] motion-safe:duration-200",
          ready ? "opacity-100" : "invisible opacity-0",
        )}
        style={{ top: position.top, left: position.left, pointerEvents: "auto" }}
      >
        <div className="flex items-start gap-2">
          <h2 id={titleId} className="min-w-0 flex-1 text-sm font-semibold text-foreground">
            {current.title}
          </h2>
          <Button
            variant="ghost"
            size="icon-xs"
            className="-mt-1 -mr-1"
            aria-label={t("Skip tour", "guided tour: button that ends the tour")}
            title={t("Skip tour", "guided tour: button that ends the tour")}
            onClick={() => onStop()}
          >
            <XIcon />
          </Button>
        </div>
        <div id={bodyId} className="text-sm text-muted-foreground">
          {typeof current.body === "string" ? (
            <Markdown variant="chat">{current.body}</Markdown>
          ) : (
            current.body
          )}
        </div>
        <div className="mt-1 flex items-center gap-2">
          <span className="flex-1 text-[11px] text-muted-foreground tabular-nums">
            {t("{current} of {total}", "guided tour: step counter, e.g. 2 of 5", {
              current: step + 1,
              total,
            })}
          </span>
          {step > 0 && (
            <Button variant="ghost" size="sm" onClick={() => goBack()}>
              {t("Back", "guided tour: previous step")}
            </Button>
          )}
          <Button size="sm" onClick={() => goNext()}>
            {isLast
              ? t("Done", "guided tour: finish the last step")
              : t("Next", "guided tour: next step")}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

/**
 * Four transparent panes around the cutout block clicks outside it. A fifth
 * over the cutout blocks the target too, unless the step lets clicks through.
 */
function ClickBlockers({ cutout, passThrough }: { cutout: Box; passThrough: boolean }) {
  const bottom = cutout.top + cutout.height;
  const right = cutout.left + cutout.width;
  const panes: React.CSSProperties[] = [
    { top: 0, left: 0, right: 0, height: Math.max(cutout.top, 0) },
    { top: bottom, left: 0, right: 0, bottom: 0 },
    { top: cutout.top, left: 0, width: Math.max(cutout.left, 0), height: cutout.height },
    { top: cutout.top, left: right, right: 0, height: cutout.height },
  ];
  if (!passThrough) {
    panes.push({ top: cutout.top, left: cutout.left, width: cutout.width, height: cutout.height });
  }
  return (
    <>
      {panes.map((style, i) => (
        <div
          key={i}
          aria-hidden
          data-slot="guide-blocker"
          className="absolute"
          style={{ ...style, pointerEvents: "auto" }}
        />
      ))}
    </>
  );
}

/**
 * A visible, human-like cursor for Playwright screen recordings.
 *
 * Adapted from the bowrain e2e cursor helper (bowrain/apps/web/e2e/helpers):
 * injects an SVG pointer that follows the mouse with eased motion, and a click
 * ripple. The ripple is tinted kapi-orange so clicks read clearly on the
 * recorded video. Everything lives in the page DOM, so it is captured by
 * Playwright's recordVideo.
 */
import type { Page, Locator } from "playwright";

/** kapi accent used for the click ripple (matches the harness KAPI orange). */
const KAPI_ORANGE = "255,122,69";

export async function injectCursor(page: Page): Promise<void> {
  await page.addStyleTag({
    content: `
      #pw-cursor {
        width: 26px; height: 26px; position: fixed; top: 0; left: 0;
        z-index: 2147483647; pointer-events: none; will-change: transform;
      }
      #pw-cursor svg { width: 100%; height: 100%;
        filter: drop-shadow(1px 2px 3px rgba(0,0,0,0.35)); }
      #pw-cursor.down { transform: scale(0.82); }
      .pw-ripple {
        position: fixed; pointer-events: none; z-index: 2147483646;
        width: 40px; height: 40px; border-radius: 50%;
        background: radial-gradient(circle, rgba(${KAPI_ORANGE},0.55) 0%, rgba(${KAPI_ORANGE},0.22) 50%, rgba(${KAPI_ORANGE},0) 70%);
        border: 2px solid rgba(${KAPI_ORANGE},0.5);
        transform: translate(-50%, -50%) scale(0);
        animation: pw-ripple 0.55s ease-out forwards;
      }
      @keyframes pw-ripple {
        0%   { transform: translate(-50%,-50%) scale(0);   opacity: 1; }
        60%  { opacity: 0.7; }
        100% { transform: translate(-50%,-50%) scale(2.2); opacity: 0; }
      }
    `,
  });

  await page.evaluate(() => {
    const c = document.createElement("div");
    c.id = "pw-cursor";
    c.innerHTML = `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M5.5 3.21V20.8c0 .45.54.67.85.35l4.86-4.86a.5.5 0 0 1 .35-.15h6.87a.5.5 0 0 0 .35-.85L6.35 2.86a.5.5 0 0 0-.85.35z" fill="#fff" stroke="#1a1a1a" stroke-width="1.3"/>
    </svg>`;
    document.body.appendChild(c);
    const w = window as unknown as { __pw: { x: number; y: number } };
    w.__pw = { x: 120, y: 120 };
    c.style.transform = "translate(120px,120px)";
    (window as unknown as { __pwMove: (x: number, y: number) => void }).__pwMove = (x, y) => {
      w.__pw.x = x;
      w.__pw.y = y;
      c.style.transform = `translate(${x}px,${y}px)`;
    };
    (window as unknown as { __pwDown: (d: boolean) => void }).__pwDown = (d) =>
      c.classList.toggle("down", d);
    (window as unknown as { __pwRipple: (x: number, y: number) => void }).__pwRipple = (x, y) => {
      const r = document.createElement("div");
      r.className = "pw-ripple";
      r.style.left = x + "px";
      r.style.top = y + "px";
      document.body.appendChild(r);
      setTimeout(() => r.remove(), 600);
    };
  });
}

async function pos(page: Page): Promise<{ x: number; y: number }> {
  return page.evaluate(
    () => (window as unknown as { __pw?: { x: number; y: number } }).__pw ?? { x: 120, y: 120 },
  );
}

/** Move the cursor to (x,y) with eased, slightly-curved human motion. */
export async function moveTo(page: Page, x: number, y: number, durationMs = 600): Promise<void> {
  const start = await pos(page);
  const steps = Math.max(14, Math.round(durationMs / 16));
  // A small perpendicular arc makes the path feel hand-driven rather than linear.
  const dx = x - start.x;
  const dy = y - start.y;
  const dist = Math.hypot(dx, dy) || 1;
  const arc = Math.min(60, dist * 0.12);
  const nx = -dy / dist;
  const ny = dx / dist;
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    const ease = t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2; // easeInOutCubic
    const bump = Math.sin(t * Math.PI) * arc;
    const cx = start.x + dx * ease + nx * bump;
    const cy = start.y + dy * ease + ny * bump;
    await page.mouse.move(cx, cy);
    await page.evaluate(
      ([px, py]) => (window as unknown as { __pwMove: (a: number, b: number) => void }).__pwMove(px, py),
      [cx, cy],
    );
    await page.waitForTimeout(16);
  }
}

async function centerOf(page: Page, locator: Locator): Promise<{ x: number; y: number }> {
  const box = await locator.boundingBox();
  if (!box) throw new Error("element has no bounding box");
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

/**
 * Move to an element, settle (mouse held still), then fire the ripple AT the
 * moment of the click (not before — that reads oddly) and click without moving.
 */
export async function humanClick(page: Page, locator: Locator): Promise<void> {
  await locator.scrollIntoViewIfNeeded().catch(() => {});
  const { x, y } = await centerOf(page, locator);
  await moveTo(page, x, y, 650);
  // Settle: the cursor is now stationary over the target.
  await page.waitForTimeout(240);
  // Ripple + press fire together with the click — same instant, same point — so
  // the press visibly lands where the ripple blooms, with no mouse movement.
  await page.evaluate(
    ([px, py]) => {
      const w = window as unknown as {
        __pwDown: (d: boolean) => void;
        __pwRipple: (a: number, b: number) => void;
      };
      w.__pwDown(true);
      w.__pwRipple(px, py);
    },
    [x, y],
  );
  await locator.click();
  await page.evaluate(() =>
    (window as unknown as { __pwDown: (d: boolean) => void }).__pwDown(false),
  );
  await page.waitForTimeout(300);
}

/**
 * Move to a text field and type key-by-key, like a person. Set `submit` to press
 * Enter afterwards — both the termbase filter bar and the TM search bar execute on
 * Enter (not as-you-type), so a typed query needs a submit to actually run.
 */
export async function humanType(
  page: Page,
  locator: Locator,
  text: string,
  opts: { submit?: boolean } = {},
): Promise<void> {
  await humanClick(page, locator);
  await locator.pressSequentially(text, { delay: 95 });
  if (opts.submit) {
    await page.waitForTimeout(280);
    await locator.press("Enter");
  }
}

/** Idle nudge — a tiny cursor drift so static beats don't feel frozen. */
export async function idle(page: Page, ms: number): Promise<void> {
  const { x, y } = await pos(page);
  await moveTo(page, x + 14, y + 8, Math.min(500, ms / 2));
  await page.waitForTimeout(Math.max(0, ms - 500));
}

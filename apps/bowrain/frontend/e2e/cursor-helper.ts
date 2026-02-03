import type { Page } from "@playwright/test";

/**
 * Injects a visible cursor element that follows mouse movements.
 * Makes recordings look more realistic by showing where clicks happen.
 */
export async function injectCursor(page: Page) {
  await page.addStyleTag({
    content: `
      #playwright-cursor {
        width: 20px;
        height: 20px;
        position: fixed;
        top: 0;
        left: 0;
        z-index: 999999;
        pointer-events: none;
        transition: transform 0.05s ease-out;
      }
      #playwright-cursor svg {
        width: 100%;
        height: 100%;
        filter: drop-shadow(1px 1px 1px rgba(0,0,0,0.3));
      }
      #playwright-cursor.clicking {
        transform: scale(0.9);
      }
      #playwright-cursor.clicking svg {
        filter: drop-shadow(0px 0px 2px rgba(59,130,246,0.8));
      }
    `,
  });

  await page.addScriptTag({
    content: `
      (function() {
        const cursor = document.createElement('div');
        cursor.id = 'playwright-cursor';
        // macOS-style cursor SVG
        cursor.innerHTML = \`
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M5.5 3.21V20.8c0 .45.54.67.85.35l4.86-4.86a.5.5 0 0 1 .35-.15h6.87a.5.5 0 0 0 .35-.85L6.35 2.86a.5.5 0 0 0-.85.35z" fill="#fff" stroke="#000" stroke-width="1"/>
          </svg>
        \`;
        document.body.appendChild(cursor);
        
        let lastX = 0, lastY = 0;
        
        document.addEventListener('mousemove', (e) => {
          lastX = e.clientX;
          lastY = e.clientY;
          cursor.style.left = e.clientX + 'px';
          cursor.style.top = e.clientY + 'px';
        });
        
        document.addEventListener('mousedown', () => {
          cursor.classList.add('clicking');
        });
        
        document.addEventListener('mouseup', () => {
          cursor.classList.remove('clicking');
        });
        
        // Expose for programmatic updates
        window.__moveCursor = (x, y) => {
          cursor.style.left = x + 'px';
          cursor.style.top = y + 'px';
        };
      })();
    `,
  });
}

/**
 * Smoothly moves the cursor to a position (for visual effect in recordings).
 */
export async function moveCursorTo(page: Page, x: number, y: number) {
  await page.evaluate(({ x, y }) => {
    (window as any).__moveCursor?.(x, y);
  }, { x, y });
}

/**
 * Moves cursor to an element's center before clicking.
 */
export async function clickWithCursor(page: Page, selector: string) {
  const element = page.locator(selector);
  const box = await element.boundingBox();
  if (box) {
    await moveCursorTo(page, box.x + box.width / 2, box.y + box.height / 2);
    await page.waitForTimeout(100);
  }
  await element.click();
}

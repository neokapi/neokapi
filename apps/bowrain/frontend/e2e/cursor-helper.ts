import type { Page, Locator } from "@playwright/test";

/**
 * Injects a visible cursor element that follows mouse movements.
 * Makes recordings look more realistic by showing where clicks happen.
 */
export async function injectCursor(page: Page) {
  await page.addStyleTag({
    content: `
      #playwright-cursor {
        width: 24px;
        height: 24px;
        position: fixed;
        top: 0;
        left: 0;
        z-index: 999999;
        pointer-events: none;
        transition: left 0.15s cubic-bezier(0.25, 0.1, 0.25, 1), 
                    top 0.15s cubic-bezier(0.25, 0.1, 0.25, 1);
      }
      #playwright-cursor svg {
        width: 100%;
        height: 100%;
        filter: drop-shadow(1px 1px 2px rgba(0,0,0,0.3));
      }
      #playwright-cursor.clicking {
        transform: scale(0.85);
      }
      #playwright-cursor.clicking svg path {
        fill: #e8e8e8;
      }
      
      /* Click ripple effect */
      .click-ripple {
        position: fixed;
        pointer-events: none;
        z-index: 999998;
        width: 60px;
        height: 60px;
        border-radius: 50%;
        background: radial-gradient(circle, rgba(255,200,0,0.7) 0%, rgba(255,180,0,0.3) 50%, rgba(255,160,0,0) 70%);
        border: 2px solid rgba(255,180,0,0.6);
        transform: translate(-50%, -50%) scale(0);
        animation: click-ripple-anim 0.5s ease-out forwards;
      }
      
      @keyframes click-ripple-anim {
        0% {
          transform: translate(-50%, -50%) scale(0);
          opacity: 1;
        }
        50% {
          opacity: 0.8;
        }
        100% {
          transform: translate(-50%, -50%) scale(2.5);
          opacity: 0;
        }
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
            <path d="M5.5 3.21V20.8c0 .45.54.67.85.35l4.86-4.86a.5.5 0 0 1 .35-.15h6.87a.5.5 0 0 0 .35-.85L6.35 2.86a.5.5 0 0 0-.85.35z" fill="#fff" stroke="#000" stroke-width="1.2"/>
          </svg>
        \`;
        document.body.appendChild(cursor);
        
        // Track position for smooth transitions
        window.__cursorX = 100;
        window.__cursorY = 100;
        cursor.style.left = '100px';
        cursor.style.top = '100px';
        
        document.addEventListener('mousemove', (e) => {
          window.__cursorX = e.clientX;
          window.__cursorY = e.clientY;
          cursor.style.left = e.clientX + 'px';
          cursor.style.top = e.clientY + 'px';
        });
        
        document.addEventListener('mousedown', (e) => {
          cursor.classList.add('clicking');
          
          // Create click ripple
          const ripple = document.createElement('div');
          ripple.className = 'click-ripple';
          ripple.style.left = e.clientX + 'px';
          ripple.style.top = e.clientY + 'px';
          document.body.appendChild(ripple);
          
          // Remove ripple after animation
          setTimeout(() => ripple.remove(), 400);
        });
        
        document.addEventListener('mouseup', () => {
          cursor.classList.remove('clicking');
        });
        
        // Expose for programmatic smooth movement
        window.__moveCursorSmooth = (x, y) => {
          cursor.style.left = x + 'px';
          cursor.style.top = y + 'px';
          window.__cursorX = x;
          window.__cursorY = y;
        };
        
        window.__getCursorPos = () => ({ x: window.__cursorX, y: window.__cursorY });
      })();
    `,
  });
}

/**
 * Smoothly moves the cursor to a position with human-like motion.
 */
export async function moveCursorTo(page: Page, x: number, y: number, duration: number = 300) {
  const start = await page.evaluate(() => (window as any).__getCursorPos?.() || { x: 100, y: 100 });
  const steps = Math.max(10, Math.floor(duration / 16)); // ~60fps
  
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    // Ease-out cubic for natural deceleration
    const ease = 1 - Math.pow(1 - t, 3);
    const currentX = start.x + (x - start.x) * ease;
    const currentY = start.y + (y - start.y) * ease;
    
    await page.evaluate(({ x, y }) => {
      (window as any).__moveCursorSmooth?.(x, y);
    }, { x: currentX, y: currentY });
    
    await page.waitForTimeout(16);
  }
}

/**
 * Moves cursor smoothly to an element's center.
 */
export async function moveCursorToElement(page: Page, locator: Locator, duration: number = 300) {
  const box = await locator.boundingBox();
  if (box) {
    await moveCursorTo(page, box.x + box.width / 2, box.y + box.height / 2, duration);
  }
}

/**
 * Human-like click: move cursor smoothly, pause, then click with ripple effect.
 * Use force=true for elements covered by overlays (like React Flow nodes).
 */
export async function humanClick(page: Page, locator: Locator, force: boolean = false) {
  await moveCursorToElement(page, locator, 400);
  await page.waitForTimeout(100); // Brief pause before click
  
  // Get element position and trigger ripple manually
  const box = await locator.boundingBox();
  if (box) {
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.evaluate(({ x, y }) => {
      // Trigger cursor click animation
      const cursor = document.getElementById('playwright-cursor');
      if (cursor) cursor.classList.add('clicking');
      
      // Create ripple effect
      const ripple = document.createElement('div');
      ripple.className = 'click-ripple';
      ripple.style.left = x + 'px';
      ripple.style.top = y + 'px';
      document.body.appendChild(ripple);
      setTimeout(() => ripple.remove(), 400);
      
      // Remove clicking class after animation
      setTimeout(() => cursor?.classList.remove('clicking'), 150);
    }, { x, y });
  }
  
  await page.waitForTimeout(50); // Let ripple start
  await locator.click({ force });
  await page.waitForTimeout(200); // Let ripple finish
}

/**
 * Human-like typing: slower, with slight variations.
 */
export async function humanType(page: Page, locator: Locator, text: string) {
  await moveCursorToElement(page, locator, 350);
  await page.waitForTimeout(100);
  await locator.click();
  await page.waitForTimeout(150);
  // Type with variable speed (80-150ms per char)
  await locator.pressSequentially(text, { delay: 100 });
}

import type { Page } from "@playwright/test";

/**
 * Injects macOS-style floating traffic lights (borderless window look).
 * No title bar - just the three buttons in the corner.
 */
export async function injectWindowChrome(page: Page, _title?: string) {
  await page.addStyleTag({
    content: `
      /* Floating traffic lights */
      #window-traffic-lights {
        position: fixed;
        top: 12px;
        left: 12px;
        z-index: 999998;
        display: flex;
        gap: 8px;
        pointer-events: none;
      }
      
      .traffic-light {
        width: 12px;
        height: 12px;
        border-radius: 50%;
        box-shadow: 
          inset 0 0 0 0.5px rgba(0,0,0,0.15),
          0 1px 2px rgba(0,0,0,0.1);
      }
      
      .traffic-light.close {
        background: linear-gradient(180deg, #ff5f57 0%, #e0443e 100%);
      }
      
      .traffic-light.minimize {
        background: linear-gradient(180deg, #ffbd2e 0%, #dea123 100%);
      }
      
      .traffic-light.maximize {
        background: linear-gradient(180deg, #28c840 0%, #1dad2b 100%);
      }
      
      /* Subtle window shadow/border effect */
      #window-shadow {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        pointer-events: none;
        z-index: 999996;
        box-shadow: inset 0 0 0 1px rgba(0,0,0,0.08);
        border-radius: 10px;
      }
      
      /* Round corners on body for native app look */
      html {
        border-radius: 10px;
        overflow: hidden;
      }
    `,
  });

  await page.evaluate(() => {
    // Add subtle window shadow
    const shadow = document.createElement('div');
    shadow.id = 'window-shadow';
    document.body.appendChild(shadow);
    
    // Add floating traffic lights
    const lights = document.createElement('div');
    lights.id = 'window-traffic-lights';
    lights.innerHTML = `
      <div class="traffic-light close"></div>
      <div class="traffic-light minimize"></div>
      <div class="traffic-light maximize"></div>
    `;
    document.body.appendChild(lights);
  });
}

import type { Page } from "@playwright/test";

/**
 * Injects macOS-style window chrome around the page content.
 * Makes recordings look like a real desktop app.
 */
export async function injectWindowChrome(page: Page, title: string = "Bowrain") {
  await page.addStyleTag({
    content: `
      /* Window container */
      #window-chrome {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        z-index: 999998;
        pointer-events: none;
        display: flex;
        flex-direction: column;
      }
      
      /* Title bar */
      #window-titlebar {
        height: 38px;
        background: linear-gradient(180deg, #e8e8e8 0%, #d4d4d4 100%);
        border-bottom: 1px solid #b3b3b3;
        display: flex;
        align-items: center;
        padding: 0 12px;
        flex-shrink: 0;
      }
      
      /* Dark mode title bar */
      @media (prefers-color-scheme: dark) {
        #window-titlebar {
          background: linear-gradient(180deg, #3d3d3d 0%, #2d2d2d 100%);
          border-bottom: 1px solid #1a1a1a;
        }
        #window-title {
          color: #e0e0e0 !important;
        }
      }
      
      /* Traffic lights */
      .traffic-lights {
        display: flex;
        gap: 8px;
        margin-right: 16px;
      }
      
      .traffic-light {
        width: 12px;
        height: 12px;
        border-radius: 50%;
        box-shadow: inset 0 0 0 1px rgba(0,0,0,0.1);
      }
      
      .traffic-light.close {
        background: linear-gradient(180deg, #ff5f57 0%, #e33e32 100%);
      }
      
      .traffic-light.minimize {
        background: linear-gradient(180deg, #febc2e 0%, #e5a21c 100%);
      }
      
      .traffic-light.maximize {
        background: linear-gradient(180deg, #28c840 0%, #1aab29 100%);
      }
      
      /* Title */
      #window-title {
        flex: 1;
        text-align: center;
        font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", sans-serif;
        font-size: 13px;
        font-weight: 500;
        color: #4a4a4a;
        margin-right: 76px; /* Balance the traffic lights */
      }
      
      /* Window shadow effect on body */
      body {
        margin-top: 38px !important;
      }
      
      /* Outer frame */
      #window-frame {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        border: 1px solid #b3b3b3;
        border-radius: 10px;
        pointer-events: none;
        z-index: 999997;
        box-shadow: 
          0 0 0 1px rgba(0,0,0,0.1),
          0 10px 30px rgba(0,0,0,0.2),
          0 5px 15px rgba(0,0,0,0.1);
      }
      
      @media (prefers-color-scheme: dark) {
        #window-frame {
          border-color: #1a1a1a;
        }
      }
    `,
  });

  await page.evaluate((title) => {
    // Add window frame
    const frame = document.createElement('div');
    frame.id = 'window-frame';
    document.body.appendChild(frame);
    
    // Add title bar
    const chrome = document.createElement('div');
    chrome.id = 'window-chrome';
    chrome.innerHTML = `
      <div id="window-titlebar">
        <div class="traffic-lights">
          <div class="traffic-light close"></div>
          <div class="traffic-light minimize"></div>
          <div class="traffic-light maximize"></div>
        </div>
        <div id="window-title">${title}</div>
      </div>
    `;
    document.body.appendChild(chrome);
  }, title);
}

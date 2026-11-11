/**
 * The element a selector names, preferring a match that is laid out (has
 * client rects) over one that is hidden.
 */
export function queryTarget(selector: string): Element | null {
  let matches: NodeListOf<Element>;
  try {
    matches = document.querySelectorAll(selector);
  } catch {
    return null;
  }
  if (matches.length === 0) return null;
  for (const el of matches) {
    if (el.getClientRects().length > 0) return el;
  }
  return matches[0] ?? null;
}

/**
 * Resolves with the element `selector` names as soon as it is in the
 * document, or with null after `timeout` ms. It checks on every DOM mutation
 * and on a short poll, which catches elements that become visible through a
 * style change no mutation observer reports. Aborting resolves with null.
 */
export function waitForTarget(
  selector: string,
  timeout: number,
  signal?: AbortSignal,
): Promise<Element | null> {
  const found = queryTarget(selector);
  if (found || timeout <= 0) return Promise.resolve(found);
  return new Promise((resolve) => {
    let done = false;
    const finish = (el: Element | null) => {
      if (done) return;
      done = true;
      observer?.disconnect();
      clearInterval(poll);
      clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
      resolve(el);
    };
    const check = () => {
      const el = queryTarget(selector);
      if (el) finish(el);
    };
    const onAbort = () => finish(null);
    const observer = typeof MutationObserver === "undefined" ? null : new MutationObserver(check);
    observer?.observe(document.body, { childList: true, subtree: true, attributes: true });
    const poll = setInterval(check, 100);
    const timer = setTimeout(() => finish(null), timeout);
    if (signal?.aborted) finish(null);
    else signal?.addEventListener("abort", onAbort);
  });
}

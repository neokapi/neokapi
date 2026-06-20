import { useCallback, useState, type RefObject } from "react";

// useMediaTime bridges an <audio>/<video> element's playhead to React state for
// the players. It exposes the current time in milliseconds, a `timeupdate`
// handler to wire onto the element, and a `seek(ms)` that moves the element's
// playhead and mirrors it locally. When `controlled` is provided the element's
// own time is ignored and the playhead is driven externally — the hook stays
// fully testable and Storybook-drivable without real playback.
export function useMediaTime(
  ref: RefObject<HTMLMediaElement | null>,
  controlled?: number,
): { timeMs: number; onTimeUpdate: () => void; seek: (ms: number) => void } {
  const [localMs, setLocalMs] = useState(0);
  const timeMs = controlled ?? localMs;

  const onTimeUpdate = useCallback(() => {
    const el = ref.current;
    if (el) setLocalMs(el.currentTime * 1000);
  }, [ref]);

  const seek = useCallback(
    (ms: number) => {
      const el = ref.current;
      if (el) el.currentTime = ms / 1000;
      setLocalMs(ms);
    },
    [ref],
  );

  return { timeMs, onTimeUpdate, seek };
}

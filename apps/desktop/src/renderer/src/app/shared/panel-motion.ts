import { useEffect, useState } from "react";

export const panelMotionDurationMs = 500;
export const panelMotionClass =
  "duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none";

export function usePanelPresence(
  active: boolean,
  durationMs = panelMotionDurationMs,
) {
  const [rendered, setRendered] = useState(active);
  const [visible, setVisible] = useState(active);

  useEffect(() => {
    let frame = 0;
    let timeout = 0;

    if (active) {
      setRendered(true);
      frame = window.requestAnimationFrame(() => setVisible(true));
      return () => {
        window.cancelAnimationFrame(frame);
      };
    }

    setVisible(false);
    timeout = window.setTimeout(() => setRendered(false), durationMs);
    return () => {
      window.clearTimeout(timeout);
    };
  }, [active, durationMs]);

  return { rendered, visible };
}

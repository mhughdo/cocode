import {
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import { cn } from "@/lib/utils";

export function useResizableRightPanel({
  defaultWidth,
  maxWidth,
  minWidth,
}: {
  defaultWidth: number;
  maxWidth: number;
  minWidth: number;
}) {
  const [width, setWidth] = useState(defaultWidth);
  const [resizing, setResizing] = useState(false);
  const dragRef = useRef<{ startWidth: number; startX: number } | null>(null);

  useEffect(() => {
    function handlePointerMove(event: PointerEvent) {
      const drag = dragRef.current;
      if (!drag) {
        return;
      }
      const nextWidth = drag.startWidth + drag.startX - event.clientX;
      setWidth(Math.min(maxWidth, Math.max(minWidth, nextWidth)));
    }

    function handlePointerUp() {
      if (!dragRef.current) {
        return;
      }
      dragRef.current = null;
      setResizing(false);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    }

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", handlePointerUp);
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };
  }, [maxWidth, minWidth]);

  const startResize = useCallback(
    (event: ReactPointerEvent) => {
      event.preventDefault();
      dragRef.current = {
        startWidth: width,
        startX: event.clientX,
      };
      setResizing(true);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [width],
  );

  return {
    gridStyle: { "--right-panel-width": `${width}px` } as CSSProperties,
    resizing,
    startResize,
    width,
  };
}

export function ResizableRightPanelHandle({
  className,
  onPointerDown,
}: {
  className?: string;
  onPointerDown: (event: ReactPointerEvent) => void;
}) {
  return (
    <div
      aria-label="Resize right panel"
      aria-orientation="vertical"
      className={cn(
        "group absolute inset-y-0 left-0 z-10 hidden w-3 -translate-x-1/2 cursor-col-resize items-stretch justify-center xl:flex",
        className,
      )}
      role="separator"
      tabIndex={0}
      onPointerDown={onPointerDown}
    >
      <div className="bg-border/70 group-hover:bg-primary/50 h-full w-px transition-colors" />
    </div>
  );
}

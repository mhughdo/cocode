import {
  type CSSProperties,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import {
  sourceInspectorDefaultWidth,
  sourceInspectorMainMinWidth,
  sourceInspectorMaxWidth,
  sourceInspectorMinWidth,
  sourceInspectorOverlayGutter,
  sourceInspectorSideBySideMinWidth,
  sourceInspectorTransitionMs,
} from "./setup-source-preview-model";

export function useSetupSourceInspectorPanel() {
  const layoutRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [rendered, setRendered] = useState(false);
  const [visible, setVisible] = useState(false);
  const [openCount, setOpenCount] = useState(0);
  const [width, setWidth] = useState(sourceInspectorDefaultWidth);
  const [resizing, setResizing] = useState(false);

  const layoutActive = open || rendered;
  const visualOpen = open && visible;
  const layoutStyle = layoutActive
    ? ({
        "--source-inspector-width": `${width}px`,
      } as CSSProperties)
    : undefined;
  const panelStyle = layoutActive
    ? ({
        width: `min(${width}px, calc(100% - 16px))`,
      } as CSSProperties)
    : undefined;

  const clampWidth = useCallback(() => {
    const availableWidth = layoutRef.current?.clientWidth ?? window.innerWidth;
    const maxWidth = maxSourceInspectorWidth(availableWidth);
    setWidth((current) =>
      Math.min(maxWidth, Math.max(sourceInspectorMinWidth, current)),
    );
  }, []);

  const toggle = useCallback(() => {
    if (open) {
      setOpen(false);
      return;
    }
    setRendered(true);
    setVisible(false);
    setOpenCount((value) => value + 1);
    setOpen(true);
  }, [open]);

  const close = useCallback(() => {
    setOpen(false);
  }, []);

  const startResize = useCallback(() => {
    setResizing(true);
  }, []);

  useEffect(() => {
    let canceled = false;
    if (open) {
      queueMicrotask(() => {
        if (!canceled) {
          setRendered(true);
        }
      });
      const frame = window.requestAnimationFrame(() => {
        if (!canceled) {
          setVisible(true);
        }
      });
      return () => {
        canceled = true;
        window.cancelAnimationFrame(frame);
      };
    }

    queueMicrotask(() => {
      if (!canceled) {
        setVisible(false);
        setResizing(false);
      }
    });
    if (!rendered) {
      return () => {
        canceled = true;
      };
    }
    const timer = window.setTimeout(() => {
      if (!canceled) {
        setRendered(false);
      }
    }, sourceInspectorTransitionMs);
    return () => {
      canceled = true;
      window.clearTimeout(timer);
    };
  }, [open, rendered]);

  useEffect(() => {
    if (!resizing) {
      return;
    }
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    function resizeFromClientX(clientX: number) {
      const layoutBounds = layoutRef.current?.getBoundingClientRect();
      const availableWidth = layoutBounds?.width ?? window.innerWidth;
      const rightEdge = layoutBounds?.right ?? window.innerWidth;
      const maxWidth = maxSourceInspectorWidth(availableWidth);
      const nextWidth = Math.min(
        maxWidth,
        Math.max(sourceInspectorMinWidth, rightEdge - clientX),
      );
      setWidth(nextWidth);
    }

    function handlePointerMove(event: PointerEvent) {
      resizeFromClientX(event.clientX);
    }

    function handleMouseMove(event: MouseEvent) {
      resizeFromClientX(event.clientX);
    }

    function handleResizeEnd() {
      setResizing(false);
    }

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("pointerup", handleResizeEnd, { once: true });
    window.addEventListener("mouseup", handleResizeEnd, { once: true });
    return () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("pointerup", handleResizeEnd);
      window.removeEventListener("mouseup", handleResizeEnd);
    };
  }, [resizing]);

  useEffect(() => {
    clampWidth();
    window.addEventListener("resize", clampWidth);
    return () => {
      window.removeEventListener("resize", clampWidth);
    };
  }, [clampWidth]);

  useEffect(() => {
    if (open) {
      clampWidth();
    }
  }, [clampWidth, open]);

  return {
    close,
    layoutActive,
    layoutRef,
    layoutStyle,
    open,
    openCount,
    panelStyle,
    rendered,
    resizing,
    startResize,
    toggle,
    visualOpen,
  };
}

function maxSourceInspectorWidth(availableWidth: number) {
  const responsiveMax =
    availableWidth < sourceInspectorSideBySideMinWidth
      ? availableWidth - sourceInspectorOverlayGutter
      : availableWidth - sourceInspectorMainMinWidth;
  return Math.max(
    sourceInspectorMinWidth,
    Math.min(sourceInspectorMaxWidth, responsiveMax),
  );
}

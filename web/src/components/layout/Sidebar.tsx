"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { AgentList } from "../agents/AgentList";
import { useT } from "@/lib/i18n";

const MIN_WIDTH = 220;
const MAX_WIDTH = 400;
const DEFAULT_WIDTH = 260;
const SWIPE_THRESHOLD = 80;

interface SidebarProps {
  isOpen?: boolean;
  onClose?: () => void;
}

export function Sidebar({ isOpen, onClose }: SidebarProps) {
  const [width, setWidth] = useState(DEFAULT_WIDTH);
  const isResizing = useRef(false);
  const touchStartX = useRef(0);
  const mobileAsideRef = useRef<HTMLElement>(null);
  const t = useT();

  const cleanupResizeRef = useRef<(() => void) | null>(null);

  // Cleanup resize listeners on unmount
  useEffect(() => {
    return () => {
      cleanupResizeRef.current?.();
    };
  }, []);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    isResizing.current = true;

    const onMouseMove = (e: MouseEvent) => {
      if (!isResizing.current) return;
      const newWidth = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, e.clientX));
      setWidth(newWidth);
    };

    const cleanup = () => {
      isResizing.current = false;
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", cleanup);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      cleanupResizeRef.current = null;
    };

    cleanupResizeRef.current = cleanup;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", cleanup);
  }, []);

  // Swipe to close on mobile
  useEffect(() => {
    const el = mobileAsideRef.current;
    if (!el || !isOpen) return;

    const handleTouchStart = (e: TouchEvent) => {
      touchStartX.current = e.touches[0].clientX;
    };

    const handleTouchEnd = (e: TouchEvent) => {
      const deltaX = e.changedTouches[0].clientX - touchStartX.current;
      if (deltaX < -SWIPE_THRESHOLD) {
        onClose?.();
      }
    };

    el.addEventListener("touchstart", handleTouchStart, { passive: true });
    el.addEventListener("touchend", handleTouchEnd, { passive: true });
    return () => {
      el.removeEventListener("touchstart", handleTouchStart);
      el.removeEventListener("touchend", handleTouchEnd);
    };
  }, [isOpen, onClose]);

  const sidebarContent = (
    <nav role="navigation" aria-label={t("sidebar.nav")} className="flex min-h-0 flex-1 flex-col">
      <div className="px-3 py-2">
        <span id="agents-heading" className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t("agents.title")}
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-1" role="region" aria-labelledby="agents-heading">
        <AgentList />
      </div>
    </nav>
  );

  return (
    <>
      {/* Mobile overlay */}
      {isOpen && (
        <div
          role="button"
          tabIndex={0}
          aria-label={t("sidebar.close")}
          className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm md:hidden"
          onClick={onClose}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") onClose?.(); }}
        />
      )}

      {/* Desktop sidebar */}
      {isOpen && (
        <aside
          className="relative hidden min-h-0 flex-col border-r bg-muted/30 md:flex"
          style={{ width }}
        >
          {sidebarContent}
          {/* Resize handle */}
          <div
            onMouseDown={handleMouseDown}
            className="absolute right-0 top-0 z-10 h-full w-1 cursor-col-resize hover:bg-primary/20 active:bg-primary/30"
          />
        </aside>
      )}

      {/* Mobile sidebar */}
      <aside
        ref={mobileAsideRef}
        className={`fixed inset-y-0 left-0 z-50 flex w-72 flex-col border-r bg-background shadow-xl transition-transform duration-200 ease-out md:hidden ${
          isOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        {sidebarContent}
      </aside>
    </>
  );
}

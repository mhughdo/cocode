import { type ReactNode } from "react";

import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";

import { MarkdownMessage } from "../chat/markdown-message";

export type LoadingState = "idle" | "loading" | "success" | "error";

export function PanelFrame({
  actions,
  children,
  className,
  eyebrow,
  title,
  subtitle,
}: {
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  eyebrow?: string;
  title: string;
  subtitle?: string;
}) {
  return (
    <aside
      data-review-panel="true"
      className={cn(
        "border-border/70 flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border bg-white shadow-[0_1px_2px_rgb(17_18_20/0.03)]",
        className,
      )}
    >
      <div className="min-w-0 border-b bg-white px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            {eyebrow && (
              <div className="text-muted-foreground text-xs font-medium">
                {eyebrow}
              </div>
            )}
            <div className="text-base font-semibold [overflow-wrap:anywhere] break-words">
              {title}
            </div>
            {subtitle && (
              <p className="text-muted-foreground mt-1 text-xs leading-5 [overflow-wrap:anywhere] break-words">
                {subtitle}
              </p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
        </div>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex w-full max-w-full min-w-0 flex-col gap-4 overflow-x-hidden p-4">
          {children}
        </div>
      </ScrollArea>
    </aside>
  );
}

export function Section({
  title,
  children,
  description,
}: {
  title: string;
  children: ReactNode;
  description?: string;
}) {
  return (
    <section className="border-border/70 max-w-full min-w-0 overflow-hidden rounded-lg border bg-white">
      <div className="px-3 py-2.5">
        <div className="text-sm font-semibold">{title}</div>
        {description && (
          <p className="text-muted-foreground mt-1 text-xs leading-5 [overflow-wrap:anywhere] break-words">
            {description}
          </p>
        )}
      </div>
      <div className="max-w-full min-w-0 border-t px-3 py-3">{children}</div>
    </section>
  );
}

export function PanelMarkdown({
  children,
  className,
}: {
  children?: string;
  className?: string;
}) {
  if (!children?.trim()) {
    return null;
  }
  return (
    <MarkdownMessage
      className={cn(
        "text-muted-foreground text-sm leading-6 [overflow-wrap:anywhere] break-words",
        className,
      )}
      content={children}
    />
  );
}

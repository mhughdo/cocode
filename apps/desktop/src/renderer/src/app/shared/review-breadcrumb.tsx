import { Fragment } from "react";
import { ChevronDownIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export type ReviewBreadcrumbItem = {
  label: string;
  onClick?: () => void;
};

export function ReviewBreadcrumb({ items }: { items: ReviewBreadcrumbItem[] }) {
  return (
    <nav
      aria-label="Review breadcrumb"
      className="text-muted-foreground mb-2 flex min-w-0 flex-wrap items-center gap-1.5 text-sm"
    >
      {items.map((item, index) => {
        const isLast = index === items.length - 1;
        return (
          <Fragment key={`${item.label}:${index}`}>
            {index > 0 ? (
              <ChevronDownIcon className="size-3.5 shrink-0 -rotate-90" />
            ) : null}
            {item.onClick && !isLast ? (
              <button
                className="hover:text-foreground focus-visible:ring-ring min-w-0 cursor-pointer truncate rounded-sm px-0.5 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none"
                type="button"
                onClick={item.onClick}
              >
                {item.label}
              </button>
            ) : (
              <span
                className={cn(
                  "min-w-0 truncate px-0.5",
                  isLast && "text-foreground",
                )}
              >
                {item.label}
              </span>
            )}
          </Fragment>
        );
      })}
    </nav>
  );
}

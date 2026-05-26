import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";

import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-4xl border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-all focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none [&>svg]:size-3!",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground [a]:hover:bg-primary/80",
        secondary:
          "bg-secondary text-secondary-foreground [a]:hover:bg-secondary/80",
        destructive:
          "bg-destructive/10 text-destructive focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:focus-visible:ring-destructive/40 [a]:hover:bg-destructive/20",
        outline:
          "border-border text-foreground [a]:hover:bg-muted [a]:hover:text-muted-foreground",
        ghost:
          "hover:bg-muted hover:text-muted-foreground dark:hover:bg-muted/50",
        link: "text-primary underline-offset-4 hover:underline",

        "severity-blocker":
          "bg-severity-blocker-surface border-severity-blocker-border text-severity-blocker-foreground",
        "severity-high":
          "bg-severity-high-surface border-severity-high-border text-severity-high-foreground",
        "severity-medium":
          "bg-severity-medium-surface border-severity-medium-border text-severity-medium-foreground",
        "severity-low":
          "bg-severity-low-surface border-severity-low-border text-severity-low-foreground",

        "status-verified":
          "bg-status-verified-surface border-status-verified-border text-status-verified-foreground",
        "status-triage":
          "bg-status-triage-surface border-status-triage-border text-status-triage-foreground",
        "status-accepted":
          "bg-status-accepted-surface border-status-accepted-border text-status-accepted-foreground",
        "status-dismissed":
          "bg-status-dismissed-surface border-status-dismissed-border text-status-dismissed-foreground",

        "signal-agent":
          "bg-signal-agent-surface border-signal-agent-border text-signal-agent-foreground",
        "signal-trace":
          "bg-signal-trace-surface border-signal-trace-border text-signal-trace-foreground",
        "signal-tool":
          "bg-signal-tool-surface border-signal-tool-border text-signal-tool-foreground",
        "signal-output":
          "bg-signal-output-surface border-signal-output-border text-signal-output-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

export type BadgeVariant = NonNullable<
  VariantProps<typeof badgeVariants>["variant"]
>;

function Badge({
  className,
  variant = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span";

  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  );
}

export { Badge, badgeVariants };

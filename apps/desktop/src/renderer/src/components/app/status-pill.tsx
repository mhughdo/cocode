import { ChevronDownIcon } from "lucide-react";

import { Badge, type BadgeVariant } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

export type FindingDecisionStatus =
  | "pending"
  | "accepted"
  | "dismissed"
  | "deferred";

const STATUS_DISPLAY: Record<
  FindingDecisionStatus,
  { label: string; variant: BadgeVariant }
> = {
  pending: { label: "Needs triage", variant: "status-triage" },
  accepted: { label: "Accepted", variant: "status-verified" },
  dismissed: { label: "Dismissed", variant: "status-dismissed" },
  deferred: { label: "Deferred", variant: "status-accepted" },
};

const ALL_OPTIONS: FindingDecisionStatus[] = [
  "pending",
  "accepted",
  "dismissed",
  "deferred",
];

export interface StatusPillProps {
  value: FindingDecisionStatus;
  onChange: (next: FindingDecisionStatus) => void;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
  options?: FindingDecisionStatus[];
}

export function StatusPill({
  value,
  onChange,
  disabled,
  className,
  ariaLabel,
  options = ALL_OPTIONS,
}: StatusPillProps) {
  const current = STATUS_DISPLAY[value];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <button
          type="button"
          aria-label={ariaLabel ?? `Status: ${current.label}`}
          className={cn(
            "focus-visible:ring-ring inline-flex items-center rounded-4xl outline-none focus-visible:ring-2 focus-visible:ring-offset-1 disabled:pointer-events-none disabled:opacity-50",
            className,
          )}
          onClick={(event) => event.stopPropagation()}
        >
          <Badge variant={current.variant} className="cursor-pointer gap-1 pr-1.5">
            <span className="truncate">{current.label}</span>
            <ChevronDownIcon className="size-3 opacity-70" />
          </Badge>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-44">
        {options.map((opt) => {
          const cfg = STATUS_DISPLAY[opt];
          const active = opt === value;
          return (
            <DropdownMenuItem
              key={opt}
              data-active={active}
              onSelect={() => onChange(opt)}
              className={cn(active && "bg-muted")}
            >
              <Badge variant={cfg.variant}>{cfg.label}</Badge>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

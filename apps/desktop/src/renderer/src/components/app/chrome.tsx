import type { ComponentProps, ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { InboxIcon, LoaderCircleIcon, TriangleAlertIcon } from "lucide-react";

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export interface AppShellProps {
  sidebar: ReactNode;
  header: ReactNode;
  children: ReactNode;
  detailPane?: ReactNode;
  statusBanner?: ReactNode;
}

export function AppShell({
  sidebar,
  header,
  children,
  detailPane,
  statusBanner,
}: AppShellProps) {
  return (
    <TooltipProvider>
      <main className="bg-background text-foreground flex min-h-screen">
        <aside className="bg-sidebar text-sidebar-foreground flex w-[244px] shrink-0 flex-col">
          {sidebar}
        </aside>
        <section className="bg-background flex min-w-0 flex-1 flex-col">
          {header}
          {statusBanner}
          <div
            className={cn(
              "grid min-h-0 flex-1",
              detailPane
                ? "grid-cols-[minmax(0,1fr)_minmax(430px,42vw)]"
                : "grid-cols-1",
            )}
          >
            {children}
            {detailPane}
          </div>
        </section>
      </main>
    </TooltipProvider>
  );
}

export interface SidebarSectionProps {
  title: string;
  action?: ReactNode;
  children: ReactNode;
}

export function SidebarSection({
  title,
  action,
  children,
}: SidebarSectionProps) {
  return (
    <section className="mt-5 flex flex-col gap-1 px-2">
      <div className="text-sidebar-muted flex h-7 items-center justify-between gap-2 px-2 text-xs">
        <span className="truncate">{title}</span>
        {action}
      </div>
      {children}
    </section>
  );
}

export interface SidebarNavButtonProps extends ComponentProps<"button"> {
  icon?: LucideIcon;
  label: string;
  meta?: ReactNode;
  active?: boolean;
}

export function SidebarNavButton({
  icon: Icon,
  label,
  meta,
  active = false,
  className,
  ...props
}: SidebarNavButtonProps) {
  return (
    <button
      className={cn(
        "text-sidebar-foreground/85 hover:bg-background/45 flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-sm transition-colors",
        active && "bg-background/60 text-sidebar-foreground",
        className,
      )}
      type="button"
      {...props}
    >
      {Icon && <Icon />}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {meta && (
        <span className="text-sidebar-muted shrink-0 text-xs">{meta}</span>
      )}
    </button>
  );
}

export interface PaneHeaderProps {
  icon?: LucideIcon;
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}

export function PaneHeader({
  icon: Icon,
  title,
  description,
  actions,
  className,
}: PaneHeaderProps) {
  return (
    <div
      className={cn(
        "bg-surface-raised flex h-12 shrink-0 items-center justify-between gap-3 border-b px-4",
        className,
      )}
    >
      <div className="flex min-w-0 items-center gap-2 text-sm">
        {Icon && <Icon />}
        <div className="min-w-0">
          <div className="truncate font-medium">{title}</div>
          {description && (
            <div className="text-muted-foreground truncate text-xs">
              {description}
            </div>
          )}
        </div>
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}

export function TooltipIconButton({
  label,
  children,
  ...props
}: ComponentProps<typeof Button> & {
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button aria-label={label} {...props}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export interface SearchCommand {
  title: string;
  description?: string;
  shortcut?: string;
  icon?: LucideIcon;
  onSelect?: () => void;
}

export interface SearchCommandGroup {
  heading: string;
  commands: SearchCommand[];
}

export function SearchCommandDialog({
  open,
  onOpenChange,
  groups,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groups: SearchCommandGroup[];
}) {
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Search cocode"
      description="Search reviews, repositories, settings, and actions."
    >
      <Command>
        <CommandInput placeholder="Search reviews, repos, files, actions..." />
        <CommandList>
          <CommandEmpty>No matching results.</CommandEmpty>
          {groups.map((group, index) => (
            <CommandGroup key={group.heading} heading={group.heading}>
              {group.commands.map((command) => (
                <CommandItem
                  key={command.title}
                  onSelect={() => {
                    command.onSelect?.();
                    onOpenChange(false);
                  }}
                >
                  {command.icon && <command.icon />}
                  <div className="min-w-0 flex-1">
                    <div className="truncate">{command.title}</div>
                    {command.description && (
                      <div className="text-muted-foreground truncate text-xs">
                        {command.description}
                      </div>
                    )}
                  </div>
                  {command.shortcut && (
                    <CommandShortcut>{command.shortcut}</CommandShortcut>
                  )}
                </CommandItem>
              ))}
              {index < groups.length - 1 && <CommandSeparator />}
            </CommandGroup>
          ))}
        </CommandList>
      </Command>
    </CommandDialog>
  );
}

export function EmptyState({
  title,
  description,
  action,
  icon: Icon = InboxIcon,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  icon?: LucideIcon;
  className?: string;
}) {
  return (
    <Empty className={className}>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Icon />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}

export function ErrorState({
  title,
  description,
  action,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <Alert variant="destructive" className={className}>
      <TriangleAlertIcon />
      <AlertTitle>{title}</AlertTitle>
      {description && <AlertDescription>{description}</AlertDescription>}
      {action && <AlertAction>{action}</AlertAction>}
    </Alert>
  );
}

export function LoadingRows({
  rows = 3,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {Array.from({ length: rows }, (_, index) => (
        <div key={index} className="flex items-center gap-3">
          <Skeleton className="size-8 shrink-0" />
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <Skeleton className="h-3 w-2/3" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ))}
      <div className="text-muted-foreground flex items-center gap-2 text-xs">
        <LoaderCircleIcon className="animate-spin" />
        Loading latest local state
      </div>
    </div>
  );
}

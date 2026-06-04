import {
  useEffect,
  useState,
  type ComponentProps,
  type CSSProperties,
  type ReactNode,
} from "react";
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
import { panelMotionClass, usePanelPresence } from "@/app/shared/panel-motion";
import { cn } from "@/lib/utils";

export interface AppShellProps {
  sidebar: ReactNode;
  header: ReactNode;
  children: ReactNode;
  detailPane?: ReactNode;
  detailPaneResizing?: boolean;
  detailPaneStyle?: CSSProperties;
  sidebarCollapsed?: boolean;
  statusBanner?: ReactNode;
}

export function AppShell({
  sidebar,
  header,
  children,
  detailPane,
  detailPaneResizing = false,
  detailPaneStyle,
  sidebarCollapsed = false,
  statusBanner,
}: AppShellProps) {
  const [sidebarPreviewOpen, setSidebarPreviewOpen] = useState(false);
  const sidebarPresence = usePanelPresence(!sidebarCollapsed);
  const sidebarPreviewPresence = usePanelPresence(
    sidebarCollapsed && sidebarPreviewOpen,
  );
  const detailPanePresence = usePanelPresence(Boolean(detailPane));
  const [renderedDetailPane, setRenderedDetailPane] =
    useState<ReactNode>(detailPane);
  const [renderedDetailPaneStyle, setRenderedDetailPaneStyle] = useState<
    CSSProperties | undefined
  >(detailPaneStyle);

  useEffect(() => {
    if (!detailPane) {
      return;
    }
    setRenderedDetailPane(detailPane);
    setRenderedDetailPaneStyle(detailPaneStyle);
  }, [detailPane, detailPaneStyle]);

  useEffect(() => {
    if (detailPanePresence.rendered) {
      return;
    }
    setRenderedDetailPane(undefined);
    setRenderedDetailPaneStyle(undefined);
  }, [detailPanePresence.rendered]);

  const detailPaneRendered = detailPanePresence.rendered && renderedDetailPane;
  const shellStyle = {
    "--app-sidebar-width": "266px",
    gridTemplateColumns: sidebarPresence.visible
      ? "var(--app-sidebar-width) minmax(0, 1fr)"
      : "0px minmax(0, 1fr)",
  } as CSSProperties;
  const detailGridStyle = detailPaneRendered
    ? ({
        ...renderedDetailPaneStyle,
        gridTemplateColumns: detailPanePresence.visible
          ? "minmax(0, 1fr) minmax(0, var(--right-panel-width, 44vw))"
          : "minmax(0, 1fr) 0px",
      } as CSSProperties)
    : undefined;

  return (
    <TooltipProvider>
      <main
        className={cn(
          "bg-background text-foreground relative grid h-screen min-h-0 overflow-hidden transition-[grid-template-columns]",
          panelMotionClass,
        )}
        style={shellStyle}
      >
        {sidebarPresence.rendered && (
          <aside
            aria-hidden={!sidebarPresence.visible}
            className={cn(
              "bg-sidebar text-sidebar-foreground border-border-subtle col-start-1 row-start-1 min-h-0 min-w-0 transform-gpu overflow-hidden border-r transition-[opacity,transform] will-change-transform",
              panelMotionClass,
              sidebarPresence.visible
                ? "translate-x-0 opacity-100"
                : "pointer-events-none -translate-x-6 opacity-0",
            )}
          >
            <div className="flex h-full w-[266px] flex-col">{sidebar}</div>
          </aside>
        )}
        {sidebarCollapsed && (
          <>
            <div
              aria-hidden="true"
              className="app-no-drag fixed inset-y-0 left-0 z-40 w-2"
              onMouseEnter={() => setSidebarPreviewOpen(true)}
            />
            {sidebarPreviewPresence.rendered && (
              <aside
                aria-hidden={!sidebarPreviewPresence.visible}
                className={cn(
                  "app-no-drag bg-sidebar text-sidebar-foreground border-border-subtle fixed inset-y-0 left-0 z-50 flex w-[266px] transform-gpu flex-col border-r shadow-2xl transition-[opacity,transform] will-change-transform",
                  panelMotionClass,
                  sidebarPreviewPresence.visible
                    ? "translate-x-0 opacity-100"
                    : "pointer-events-none -translate-x-6 opacity-0",
                )}
                onMouseEnter={() => setSidebarPreviewOpen(true)}
                onMouseLeave={() => setSidebarPreviewOpen(false)}
              >
                {sidebar}
              </aside>
            )}
          </>
        )}
        <section className="bg-background col-start-2 row-start-1 flex min-h-0 min-w-0 flex-1 flex-col">
          {header}
          {statusBanner}
          <div
            style={detailGridStyle}
            className={cn(
              "grid min-h-0 flex-1 transition-[grid-template-columns]",
              detailPaneResizing ? "transition-none" : panelMotionClass,
              !detailPaneRendered && "grid-cols-1",
            )}
          >
            {children}
            {detailPaneRendered && (
              <div
                aria-hidden={!detailPanePresence.visible}
                className={cn(
                  "h-full min-h-0 min-w-0 transform-gpu overflow-hidden transition-[opacity,transform] will-change-transform",
                  panelMotionClass,
                  detailPanePresence.visible
                    ? "translate-x-0 opacity-100"
                    : "pointer-events-none translate-x-8 opacity-0",
                )}
              >
                {renderedDetailPane}
              </div>
            )}
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
    <section className="mt-5 flex flex-col gap-1 px-3">
      <div className="text-sidebar-muted flex h-7 items-center justify-between gap-2 px-2 text-[0.7rem] font-medium tracking-[0.04em] uppercase">
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
        "text-sidebar-foreground/80 hover:bg-surface-muted/70 hover:text-sidebar-foreground flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-[0.84rem] transition-colors [&_svg]:size-3.5 [&_svg]:shrink-0",
        active && "bg-surface-muted text-sidebar-foreground font-medium",
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
        "bg-background border-border-subtle flex h-14 shrink-0 items-center justify-between gap-4 border-b px-5",
        className,
      )}
    >
      <div className="flex min-w-0 items-center gap-3 text-sm">
        {Icon && <Icon />}
        <div className="min-w-0">
          <div className="truncate text-[0.92rem] font-semibold">{title}</div>
          {description && (
            <div className="text-muted-foreground mt-0.5 truncate text-xs">
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
      description="Search threads, projects, settings, and actions."
    >
      <Command>
        <CommandInput placeholder="Search threads, projects, files, actions..." />
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
